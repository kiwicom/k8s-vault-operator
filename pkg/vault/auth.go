package vault

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	vaultApi "github.com/hashicorp/vault/api"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Tokener interface {
	Token() (string, error)
}

type AuthToken struct {
	token string
}

func NewAuthToken(token string) AuthToken {
	return AuthToken{token: token}
}

func (a AuthToken) Token() (string, error) {
	return a.token, nil
}

type AuthServiceAccount struct {
	name             string
	namespace        string
	role             string
	cachedDuration   time.Duration
	path             string
	vaultClient      *vaultApi.Client
	autoMount        bool
	k8ClientSet      *kubernetes.Clientset
	cacheMx          sync.RWMutex
	cachedVaultToken string
	vaultTokenExpire time.Time
}

func NewAuthServiceAccount(vaultClient *vaultApi.Client, k8ClientSet *kubernetes.Clientset,
	name, namespace, role, path string, automount bool, cachedDuration time.Duration) *AuthServiceAccount {
	return &AuthServiceAccount{
		name:           name,
		namespace:      namespace,
		role:           role,
		path:           path,
		vaultClient:    vaultClient,
		autoMount:      automount,
		k8ClientSet:    k8ClientSet,
		cachedDuration: cachedDuration,
	}
}
func (a *AuthServiceAccount) cachedToken() string {
	a.cacheMx.RLock()
	defer a.cacheMx.RUnlock()
	return a.cachedVaultToken
}

func (a *AuthServiceAccount) Token() (string, error) {
	vaultToken := a.cachedToken()
	if vaultToken != "" && time.Now().Add(a.cachedDuration).Before(a.vaultTokenExpire) {
		return vaultToken, nil
	}

	jwtToken, err := a.fetchJWT()
	if err != nil {
		return "", fmt.Errorf("could not fetch JWT token: %w", err)
	}

	data := map[string]any{
		"role": a.role,
		"jwt":  jwtToken,
	}

	t := time.Now()
	resp, err := a.vaultClient.Logical().Write(a.path, data)
	if err != nil {
		return "", fmt.Errorf("failed to login to Vault with JWT: %w", err)
	}

	token, err := resp.TokenID()
	if err != nil {
		return "", fmt.Errorf("could not read auth token from response: %w", err)
	}

	duration, err := resp.TokenTTL()
	if err != nil {
		return "", fmt.Errorf("could not read auth token TTL from response: %w", err)
	}
	expireIn := t.Add(duration)

	a.cacheMx.Lock()
	defer a.cacheMx.Unlock()
	a.cachedVaultToken = token
	a.vaultTokenExpire = expireIn

	return token, nil
}

func (a *AuthServiceAccount) fetchJWT() (string, error) {
	if a.autoMount {
		return fetchJWTFromAutoMountedSecret()
	}

	if a.k8ClientSet == nil {
		return "", fmt.Errorf("not defined k8s clientset")
	}

	tr := &authenticationv1.TokenRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.name,
			Namespace: a.namespace,
		},
	}

	res, err := a.k8ClientSet.CoreV1().ServiceAccounts(a.namespace).
		CreateToken(context.TODO(), a.name, tr, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	return res.Status.Token, nil
}

func fetchJWTFromAutoMountedSecret() (string, error) {
	const autoMountPath = "/run/secrets/kubernetes.io/serviceaccount/token"

	if _, err := os.Stat(autoMountPath); os.IsNotExist(err) {
		return "", fmt.Errorf("file at path %q does not exists", autoMountPath)
	}

	token, err := os.ReadFile(autoMountPath)
	if err != nil {
		return "", fmt.Errorf("could not read JWT Token from path %q: %w", autoMountPath, err)
	}

	return string(token), nil
}
