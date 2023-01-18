package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	v1 "github.com/kiwicom/k8s-vault-operator/api/v1"
)

func readYaml(path string) (*v1.VaultSecret, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read file (%s): %w", path, err)
	}

	vs := v1.VaultSecret{}

	err = yaml.Unmarshal(b, &vs)
	if err != nil {
		return nil, fmt.Errorf("could not parse YAML: %w", err)
	}

	return &vs, nil
}
