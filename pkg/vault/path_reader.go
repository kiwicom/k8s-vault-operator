package vault

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/vault/api"
)

type PathReader struct {
	Client          *api.Client
	log             logr.Logger
	reconcilePeriod time.Duration
}

var (
	ErrNotFound = errors.New("path doesn't exist or is empty")
)

func (r *PathReader) Read(path string) (map[string]any, error) {
	var (
		data map[string]any
		err  error
	)
	mountPath, pathType, err := kvPreflightVersionRequest(r.Client, path)

	if err != nil {
		return nil, err
	}

	switch pathType {
	case "kv":
		data, err = r.readKV2(path, mountPath)
	default:
		return nil, fmt.Errorf("unsupported secret engine %q", pathType)
	}

	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	return data, nil
}

func (r *PathReader) readKV2(path, mountPath string) (map[string]any, error) {
	apiPath := addPrefixToVKVPath(path, mountPath, "data")

	secret, err := r.Client.Logical().Read(apiPath)
	if err != nil {
		return nil, fmt.Errorf("could not read Vault secrets at path '%s': %w", path, err)
	}

	if secret == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, path)
	}

	if secret.Data["data"] != nil {
		return secret.Data["data"].(map[string]any), nil
	}

	return make(map[string]any), nil
}
