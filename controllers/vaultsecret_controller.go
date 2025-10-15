/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	vaultAPI "github.com/hashicorp/vault/api"
	corev1 "k8s.io/api/core/v1"
	k8Errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	k8skiwicomv1 "github.com/kiwicom/k8s-vault-operator/api/v1"
	operatorMetrics "github.com/kiwicom/k8s-vault-operator/pkg/metrics"
	"github.com/kiwicom/k8s-vault-operator/pkg/vault"
)

// VaultSecretReconciler reconciles a VaultSecret object
type VaultSecretReconciler struct {
	Client        client.Client
	Scheme        *runtime.Scheme
	EventRecorder *EventRecorder
	VaultConfig   vault.AppConfig
	VaultClient   *vaultAPI.Client
	K8ClientSet   *kubernetes.Clientset
	authSACache   map[string]*vault.AuthServiceAccount
	saCacheMx     sync.RWMutex
}

func NewVaultReconciler(mgr manager.Manager, cfg vault.AppConfig, vaultClient *vaultAPI.Client,
	k8ClientSet *kubernetes.Clientset) (*VaultSecretReconciler, error) {
	reconciler := &VaultSecretReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		EventRecorder: &EventRecorder{Recorder: mgr.GetEventRecorderFor("vault-operator")},
		VaultConfig:   cfg,
		VaultClient:   vaultClient,
		K8ClientSet:   k8ClientSet,
		authSACache:   make(map[string]*vault.AuthServiceAccount),
	}
	err := reconciler.setupWithManager(mgr)
	if err != nil {
		return nil, err
	}
	return reconciler, nil
}

//+kubebuilder:rbac:groups=k8s.kiwi.com,resources=vaultsecrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=k8s.kiwi.com,resources=vaultsecrets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=k8s.kiwi.com,resources=vaultsecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VaultSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
// nolint:nonamedreturns
func (r *VaultSecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	logger := ctrl.LoggerFrom(ctx)
	labels := map[string]string{"namespace": req.Namespace, "name": req.Name, "error": "false"}
	startedAt := time.Now()

	logger.Info("Reconciling VaultSecret")
	defer logger.Info("Finished reconciling VaultSecret")

	// Fetch the VaultSecret instance
	var vaultSecret k8skiwicomv1.VaultSecret
	if err := r.Client.Get(ctx, req.NamespacedName, &vaultSecret); err != nil {
		if k8Errors.IsNotFound(err) {
			logger.Info("VaultSecret has been removed")
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			// TODO use client.IgnoreNotFound(err) ?
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if err := r.validateResource(&vaultSecret, req); err != nil {
		r.EventRecorder.Warning(&vaultSecret, "invalid resource", err)
		// since the resource is invalid, and it can't be magically fixed, someone has to manually fix it
		// so no re-queuing
		logger.Error(err, "validate")
		return ctrl.Result{}, nil
	}

	// no need to check error, because this step is validated in validateResource func
	reconcileAfter, _ := time.ParseDuration(vaultSecret.Spec.ReconcilePeriod)

	// monitoring only reconcile loops, which happen after here
	defer func() {
		if err != nil {
			labels["error"] = "true"
		}

		operatorMetrics.ReconcileCount.With(labels).Inc()
		operatorMetrics.ReconcileDuration.With(labels).Observe(time.Since(startedAt).Seconds())
	}()

	var tokener vault.Tokener
	if vaultSecret.Spec.Auth.Token != "" {
		tokener = vault.NewAuthToken(vaultSecret.Spec.Auth.Token)
	} else {
		saAccount, err := r.getAuthServiceAccount(vaultSecret)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("get auth service account: %w", err)
		}
		tokener = saAccount
	}
	reader, err := vault.NewReader(tokener, &vaultSecret, logger, &r.VaultConfig)
	if err != nil {
		r.EventRecorder.Warning(&vaultSecret, "vault failed", err)
		return ctrl.Result{}, err
	}

	if err := reader.ReadData(ctx); err != nil {
		r.EventRecorder.Warning(&vaultSecret, "vault read failed", err)
		return ctrl.Result{}, err
	}

	// Determine UI base URL: prefer config value, fallback to deriving from vaultSecret.Spec.Addr
	uiBaseAddr := r.VaultConfig.VaultUIAddr
	if uiBaseAddr == "" && vaultSecret.Spec.Addr != "" {
		uiBaseAddr = strings.TrimSuffix(vaultSecret.Spec.Addr, "/") + "/ui"
	}

	k8sSecret, err := vault.NewSecret(ctx, &vaultSecret, reader.GetData(), uiBaseAddr)
	if err != nil {
		logger.Info(fmt.Sprintf("VaultOperator sync rejected: %v", err))
		r.EventRecorder.Warning(&vaultSecret, "sync rejected", err)
		return ctrl.Result{}, nil
	}

	// Set VaultSecret as the owner and controller
	if err := controllerutil.SetControllerReference(&vaultSecret, k8sSecret, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	// Check if this Secret already exists
	var found corev1.Secret
	err = r.Client.Get(ctx, types.NamespacedName{Name: k8sSecret.Name, Namespace: k8sSecret.Namespace}, &found)
	if err != nil {
		if !k8Errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		logger.Info("Creating a new Secret Secret.namespace: " + k8sSecret.Namespace + " Secret.name: " + k8sSecret.Name)
		err = r.Client.Create(ctx, k8sSecret)
		if err != nil {
			return ctrl.Result{}, err
		}
		r.EventRecorder.Normal(&vaultSecret, "created", "Secret has been created.")
	} else {
		managedBy := found.Labels["managed-by"]
		if managedBy != "" && managedBy != vault.ManagedByLabel {
			logger.Info("syncing existing secret, that was not managed by vault operator", "name", found.Name)
		}
	}

	// Secret already exists - preserve existing annotations we don't manage
	if found.Annotations != nil {
		for key, value := range found.Annotations {
			if _, exists := k8sSecret.Annotations[key]; !exists {
				k8sSecret.Annotations[key] = value
			}
		}
	}

	// Check if update is needed
	eq := reflect.DeepEqual(k8sSecret.Data, found.Data) && reflect.DeepEqual(k8sSecret.Annotations, found.Annotations)
	if !eq {
		logger.Info("Secret exists, updating: " + k8sSecret.Namespace + " Secret.name: " + k8sSecret.Name)

		err = r.Client.Update(ctx, k8sSecret)
		if err != nil {
			return ctrl.Result{}, err
		}

		r.EventRecorder.Normal(&vaultSecret, "updated", "Secret has been updated.")
	}
	if err := updateVaultSecretResource(ctx, r.Client, &vaultSecret); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: reconcileAfter}, nil
}

func (r *VaultSecretReconciler) cachedSA(id string) *vault.AuthServiceAccount {
	r.saCacheMx.RLock()
	defer r.saCacheMx.RUnlock()
	return r.authSACache[id]
}

func (r *VaultSecretReconciler) getAuthServiceAccount(vaultSecret k8skiwicomv1.VaultSecret) (*vault.AuthServiceAccount, error) {
	saRef := vaultSecret.Spec.Auth.ServiceAccountRef
	id := fmt.Sprintf("%s-%s-%s-%s-%s", vaultSecret.Spec.Addr, vaultSecret.Namespace, saRef.Name, saRef.Role, saRef.AuthPath)
	saAccount := r.cachedSA(id)
	if saAccount != nil {
		return saAccount, nil
	}

	vaultClient, err := r.VaultClient.Clone()
	if err != nil {
		return nil, fmt.Errorf("vault client clone: %w", err)
	}
	if err := vaultClient.SetAddress(vaultSecret.Spec.Addr); err != nil {
		return nil, fmt.Errorf("vault set address: %w", err)
	}
	saAccount = vault.NewAuthServiceAccount(vaultClient, r.K8ClientSet, saRef.Name, vaultSecret.Namespace, saRef.Role,
		saRef.AuthPath, false, r.VaultConfig.RefreshTokenBefore)
	r.saCacheMx.Lock()
	defer r.saCacheMx.Unlock()
	r.authSACache[id] = saAccount
	return saAccount, nil
}

// setupWithManager sets up the controller with the Manager.
func (r *VaultSecretReconciler) setupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8skiwicomv1.VaultSecret{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.VaultConfig.MaxConcurrentReconciles,
		}).
		Complete(r)
}

func (r *VaultSecretReconciler) validateResource(vaultSecret *k8skiwicomv1.VaultSecret, req reconcile.Request) error {
	if vaultSecret.Spec.TargetSecretName == "" {
		vaultSecret.Spec.TargetSecretName = vaultSecret.Name
	}

	if vaultSecret.Spec.TargetFormat == "" {
		vaultSecret.Spec.TargetFormat = vault.TypeEnv
	}

	if vaultSecret.Spec.ReconcilePeriod == "" {
		vaultSecret.Spec.ReconcilePeriod = r.VaultConfig.DefaultReconcilePeriod
	}

	_, err := time.ParseDuration(vaultSecret.Spec.ReconcilePeriod)
	if err != nil {
		return fmt.Errorf("VaultSecret.Spec.ReconcilePeriod is invalid: %w", err)
	}

	if vaultSecret.Spec.Addr == "" {
		if r.VaultConfig.DefaultVaultAddr == "" {
			return errors.New("default vault addr from app config is empty")
		}

		vaultSecret.Spec.Addr = r.VaultConfig.DefaultVaultAddr
	}

	if vaultSecret.Spec.Auth.Token != "" {
		return nil
	}

	if vaultSecret.Spec.Auth.ServiceAccountRef == nil {
		vaultSecret.Spec.Auth.ServiceAccountRef = &k8skiwicomv1.VaultSecretAuthServiceAccountRefSpec{}
	}

	if vaultSecret.Spec.Auth.ServiceAccountRef.Name == "" {
		if r.VaultConfig.DefaultSAName == "" {
			return errors.New("default SA name from app config is empty")
		}
		vaultSecret.Spec.Auth.ServiceAccountRef.Name = r.VaultConfig.DefaultSAName
	}

	if vaultSecret.Spec.Auth.ServiceAccountRef.AuthPath == "" {
		if r.VaultConfig.DefaultSAAuthPath == "" {
			return errors.New("default SA auth path from app config is empty")
		}

		vaultSecret.Spec.Auth.ServiceAccountRef.AuthPath = r.VaultConfig.DefaultSAAuthPath
	}

	if vaultSecret.Spec.Auth.ServiceAccountRef.Role == "" {
		if r.VaultConfig.Role != "" {
			vaultSecret.Spec.Auth.ServiceAccountRef.Role = r.VaultConfig.Role
		} else {
			vaultSecret.Spec.Auth.ServiceAccountRef.Role = req.Namespace
		}
	}

	return nil
}

func updateVaultSecretResource(ctx context.Context, c client.Client, secret *k8skiwicomv1.VaultSecret) error {
	// only status is updated here
	secret.Status.LastUpdated = metav1.Now().Format(time.RFC3339)
	err := c.Status().Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update resource status: %w", err)
	}

	return nil
}
