package vault

import (
	"context"
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
	ErrNotFound = errors.New("path doesn't exist")
	ErrEmpty    = errors.New("path is empty")
)

func (r *PathReader) Read(ctx context.Context, path string) (map[string]any, int, error) {
	mountPath, version, err := kvPreflightVersionRequest(ctx, r.Client, path)

	if err != nil {
		return nil, 0, err
	}

	switch version {
	case 1:
	case 2:
		path = addPrefixToKVPath(path, mountPath, "data")
	default:
		return nil, 0, fmt.Errorf("unsupported secret engine version %d", version)
	}

	secret, err := kvReadRequest(ctx, r.Client, path, nil)

	if err != nil {
		return nil, 0, err
	}

	if secret == nil {
		return nil, 0, fmt.Errorf("%w: %s", ErrEmpty, path)
	}

	if version == 2 {
		if data, ok := secret.Data["data"]; ok && data != nil {
			return data.(map[string]any), version, nil
		}
		return nil, 0, fmt.Errorf("%w: %s", ErrEmpty, path)
	}

	return secret.Data, version, nil
}
