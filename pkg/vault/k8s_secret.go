package vault

import (
	"context"
	//nolint:gosec
	"crypto/sha1"
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	v1 "github.com/kiwicom/k8s-vault-operator/api/v1"
)

const (
	ManagedByLabel = "vault-secret-operator"
)

func validateEnvKey(key string) bool {
	for _, r := range key {
		// The keys of data  must consist of alphanumeric characters, -, _ or .
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '.' && r != '_' {
			return false
		}
	}
	return true
}

func secretsAsEnv(logger logr.Logger, secret *v1.VaultSecret, secrets Data) (map[string][]byte, error) {
	envSecrets, err := secrets.ENV(secret.Spec.GetSeparator())
	if err != nil {
		return nil, err
	}

	output := make(map[string][]byte, len(envSecrets))

	for key, val := range envSecrets {
		if !validateEnvKey(key) {
			logger.Error(fmt.Errorf("invalid key %q", key), "invalid key", "key", key)
			continue
		}
		output[key] = []byte(fmt.Sprintf("%v", val))
	}

	return output, nil
}

func secretsAsFile(secrets Data, format string) (map[string][]byte, error) {
	var (
		output   = map[string][]byte{}
		data     []byte
		err      error
		filename string
	)

	switch strings.ToLower(format) {
	case TypeJSON:
		data, err = secrets.JSON()
		filename = "secrets.json"
	case TypeYaml:
		data, err = secrets.Yaml()
		filename = "secrets.yaml"
	default:
		return nil, fmt.Errorf("%q is not supported as output format", format)
	}

	if err != nil {
		return nil, err
	}

	output[filename] = data

	return output, nil
}

// buildVaultUIURL constructs a Vault UI URL from a vault path
func buildVaultUIURL(uiBaseAddr, path string) string {
	// Remove trailing slashes from base address
	uiBaseAddr = strings.TrimSuffix(uiBaseAddr, "/")

	// Handle empty or invalid paths
	if path == "" {
		return uiBaseAddr
	}

	// Split path into components
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return uiBaseAddr
	}

	// Extract mount point (first component)
	mount := parts[0]

	// Check if path ends with wildcard
	isWildcard := strings.HasSuffix(path, "/*") || strings.HasSuffix(path, "*")

	if isWildcard {
		// For wildcard paths, use list view
		// Remove the wildcard from parts
		pathWithoutWildcard := strings.TrimSuffix(strings.TrimSuffix(path, "*"), "/")
		remainingPath := strings.TrimPrefix(pathWithoutWildcard, mount+"/")
		remainingPath = strings.TrimPrefix(remainingPath, mount)
		remainingPath = strings.Trim(remainingPath, "/")

		if remainingPath == "" {
			return fmt.Sprintf("%s/vault/secrets/%s/kv/list/", uiBaseAddr, mount)
		}
		return fmt.Sprintf("%s/vault/secrets/%s/kv/list/%s/", uiBaseAddr, mount, remainingPath)
	}

	// For specific paths, use show view with URL-encoded path
	remainingPath := strings.TrimPrefix(path, mount+"/")
	remainingPath = strings.TrimPrefix(remainingPath, mount)
	remainingPath = strings.Trim(remainingPath, "/")

	if remainingPath == "" {
		return fmt.Sprintf("%s/vault/secrets/%s/kv/", uiBaseAddr, mount)
	}

	// URL encode the remaining path (slashes become %2F)
	encodedPath := url.PathEscape(remainingPath)
	// PathEscape doesn't encode slashes, so we need to do it manually
	encodedPath = strings.ReplaceAll(encodedPath, "/", "%2F")

	return fmt.Sprintf("%s/vault/secrets/%s/kv/%s", uiBaseAddr, mount, encodedPath)
}

func NewSecret(ctx context.Context, vaultSecret *v1.VaultSecret, data Data, uiBaseAddr string) (*corev1.Secret, error) {
	var (
		err      error
		contents map[string][]byte
		format   = strings.ToLower(vaultSecret.Spec.TargetFormat)
	)
	logger := ctrl.LoggerFrom(ctx)

	switch format {
	case TypeYaml, TypeJSON:
		contents, err = secretsAsFile(data, format)
	case TypeEnv:
		contents, err = secretsAsEnv(logger, vaultSecret, data)
	default:
		return nil, fmt.Errorf("invalid target format: %q", format)
	}

	if err != nil {
		return nil, err
	}

	owner := vaultSecret.Name
	if len(owner) > 63 {
		//nolint:gosec
		s := sha1.New()
		s.Write([]byte(owner))
		owner = fmt.Sprintf("%x", s.Sum(nil))
	}

	labels := map[string]string{
		"owner":      owner,
		"managed-by": ManagedByLabel,
	}

	// Build annotations with Vault UI URLs
	annotations := make(map[string]string)

	// Build comma-separated list of UI URLs for all paths
	if uiBaseAddr != "" && len(vaultSecret.Spec.Paths) > 0 {
		var urls []string
		for _, pathSpec := range vaultSecret.Spec.Paths {
			url := buildVaultUIURL(uiBaseAddr, pathSpec.Path)
			urls = append(urls, url)
		}
		annotations["k8s-vault-operator/vault-ui-urls"] = strings.Join(urls, ", ")
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        vaultSecret.Spec.TargetSecretName,
			Namespace:   vaultSecret.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Type: corev1.SecretTypeOpaque,
		Data: contents,
	}, nil
}
