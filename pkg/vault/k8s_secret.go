package vault

import (
	"context"
	"fmt"
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

func NewSecret(ctx context.Context, vaultSecret *v1.VaultSecret, data Data) (*corev1.Secret, error) {
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

	labels := map[string]string{
		"owner":      vaultSecret.Name,
		"managed-by": ManagedByLabel,
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vaultSecret.Spec.TargetSecretName,
			Namespace: vaultSecret.Namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: contents,
	}, nil
}
