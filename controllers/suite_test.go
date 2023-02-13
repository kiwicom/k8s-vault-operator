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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-envparse"
	vaultAPI "github.com/hashicorp/vault/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	k8skiwicomv1 "github.com/kiwicom/k8s-vault-operator/api/v1"
	"github.com/kiwicom/k8s-vault-operator/pkg/vault"
	//nolint:gci
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	vaultClient *vaultAPI.Client
	ctx         context.Context
	cancel      func()
	casesDirs   []string
	logSink     bytes.Buffer
)

const (
	namespace = "default"
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)
	entries, err := os.ReadDir("../tests/cases")
	Expect(err).ToNot(HaveOccurred())
	for _, entry := range entries {
		casesDirs = append(casesDirs, entry.Name())
	}
	RunSpecs(t, "VaultOperator Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = k8skiwicomv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	mw := io.MultiWriter(os.Stderr, &logSink)
	opts := zap.Options{
		Development: false,
		DestWriter:  mw,
	}

	logger := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(logger)

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
		Logger: logger,
	})
	Expect(err).ToNot(HaveOccurred())

	appConfig, err := vault.NewAppConfig()
	Expect(err).ToNot(HaveOccurred())

	k8ClientSet, err := kubernetes.NewForConfig(k8sManager.GetConfig())
	Expect(err).ToNot(HaveOccurred())

	vaultClient, err = vault.NewClient(appConfig)
	Expect(err).ToNot(HaveOccurred())
	vaultClient.SetToken("testtoken")
	_, err = NewVaultReconciler(k8sManager, appConfig, vaultClient, k8ClientSet)
	Expect(err).ToNot(HaveOccurred())

	err = vaultClient.Sys().Mount("v1", &vaultAPI.MountInput{
		Type: "kv",
		Config: vaultAPI.MountConfigInput{
			Options: map[string]string{
				"version": "1",
			},
		},
		Local: true,
		Options: map[string]string{
			"version": "1",
		},
	})
	Expect(err).ToNot(HaveOccurred())
	for _, d := range v1Data {
		err = vaultClient.KVv1("v1").Put(ctx, strings.TrimPrefix(d.path, "v1/"), d.data)
		Expect(err).ToNot(HaveOccurred())
	}
	for _, d := range data {
		_, err = vaultClient.KVv2("secret").Put(ctx, strings.TrimPrefix(d.path, "secret/"), d.data)
		Expect(err).ToNot(HaveOccurred())
	}

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

})

var _ = AfterSuite(func() {
	for _, d := range data {
		err := vaultClient.KVv2("secret").Delete(ctx, strings.TrimPrefix(d.path, "secret/"))
		Expect(err).ToNot(HaveOccurred())
	}

	err := vaultClient.Sys().Unmount("v1")
	Expect(err).NotTo(HaveOccurred())

	cancel()
	By("tearing down the test environment")
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())

})

var _ = Describe("Cases", func() {
	for _, caseDir := range casesDirs {
		caseDir := caseDir
		It(caseDir, func() {
			base := "../tests/cases/" + caseDir + "/"

			var eEnv map[string]string
			// Open Env file
			fEnv, err := os.Open(base + "expected.env")
			if !os.IsNotExist(err) {
				Expect(err).NotTo(HaveOccurred())
				defer fEnv.Close()
				// Decode
				eEnv, err = envparse.Parse(fEnv)
				Expect(err).NotTo(HaveOccurred())
			}
			// Open JSON file
			var eJSON map[string]any
			fJSON, err := os.Open(base + "expected.json")
			if !os.IsNotExist(err) {
				defer fJSON.Close()
				Expect(err).NotTo(HaveOccurred())
				// Decode
				err = json.NewDecoder(fJSON).Decode(&eJSON)
				Expect(err).NotTo(HaveOccurred())
			}
			// Open error file
			var expectedError string
			eError, err := os.Open(base + "expected.error")
			if !os.IsNotExist(err) {
				defer eError.Close()
				Expect(err).NotTo(HaveOccurred())
				b, err := io.ReadAll(eError)
				Expect(err).NotTo(HaveOccurred())
				expectedError = string(b)
			}

			// VaultSecret
			fVS, err := os.Open(base + "vault_secret.yaml")
			Expect(err).NotTo(HaveOccurred())
			defer fVS.Close()

			var vs k8skiwicomv1.VaultSecret
			err = yaml.NewDecoder(fVS).Decode(&vs)
			vs.Name = caseDir
			vs.Spec.TargetSecretName = caseDir
			vs.Spec.Auth.Token = "testtoken"
			vs.Namespace = namespace
			Expect(err).NotTo(HaveOccurred())

			jsonVS := vs
			// Create ENV VaultSecret
			if eEnv != nil || expectedError != "" {
				vs.Spec.TargetFormat = "env"
				err = k8sClient.Create(ctx, &vs)
				Expect(err).ToNot(HaveOccurred())
			}
			// Create  JSON VaultSecret
			if eJSON != nil {
				jsonVS.Name += "-json"
				jsonVS.Spec.TargetFormat = "json"
				jsonVS.Spec.TargetSecretName += "-json"
				err = k8sClient.Create(ctx, &jsonVS)
				Expect(err).ToNot(HaveOccurred())
			}

			// Compare env
			if eEnv != nil {
				var secret corev1.Secret
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: vs.Namespace, Name: vs.Spec.TargetSecretName}, &secret)
					return err == nil
				}).Should(BeTrue())
				m := make(map[string]string)
				for k, v := range secret.Data {
					m[k] = string(v)
				}
				Expect(m).To(BeEquivalentTo(eEnv))
			}

			// Compare JSON
			if eJSON != nil {
				var secret corev1.Secret
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Namespace: jsonVS.Namespace, Name: jsonVS.Spec.TargetSecretName}, &secret)
					return err == nil
				}).Should(BeTrue())
				j, ok := secret.Data["secrets.json"]
				Expect(ok).To(BeTrue())
				m := make(map[string]any)
				Expect(json.Unmarshal(j, &m)).NotTo(HaveOccurred())
				Expect(m).To(BeEquivalentTo(eJSON))
			}

			if expectedError != "" {
				Eventually(func() bool {
					entries := getLogsToVSName(caseDir)
					for _, entry := range entries {
						if entry.Error == expectedError {
							return true
						}
					}
					return false
				}).Should(BeTrue())
			}

		})
	}
})
