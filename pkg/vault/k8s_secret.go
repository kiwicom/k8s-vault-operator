package vault

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/kiwicom/k8s-vault-operator/api/v1"
)

const (
	ManagedByLabel = "vault-secret-operator"
)

func secretsAsEnv(secret *v1.VaultSecret, secrets Data) (map[string][]byte, error) {
	envSecrets, err := secrets.ENV(secret.Spec.GetSeparator())
	if err != nil {
		return nil, err
	}

	output := make(map[string][]byte, len(envSecrets))

	for key, val := range envSecrets {
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

func NewSecret(vaultSecret *v1.VaultSecret, data Data) (*corev1.Secret, error) {
	var (
		err      error
		contents map[string][]byte
		format   = strings.ToLower(vaultSecret.Spec.TargetFormat)
	)

	switch format {
	case TypeYaml, TypeJSON:
		contents, err = secretsAsFile(data, format)
	case TypeEnv:
		contents, err = secretsAsEnv(vaultSecret, data)
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
