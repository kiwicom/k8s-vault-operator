package vault

import (
	"errors"
	"path"
	"strings"

	"github.com/hashicorp/vault/api"
)

// kvPreflightVersionRequest is inspired from https://github.com/hashicorp/vault/blob/master/command/kv_helpers.go#L44
func kvPreflightVersionRequest(client *api.Client, path string) (string, string, error) {
	// We don't want to use a wrapping call here so save any custom value and
	// restore after
	currentWrappingLookupFunc := client.CurrentWrappingLookupFunc()
	client.SetWrappingLookupFunc(nil)
	defer client.SetWrappingLookupFunc(currentWrappingLookupFunc)
	currentOutputCurlString := client.OutputCurlString()
	client.SetOutputCurlString(false)
	defer client.SetOutputCurlString(currentOutputCurlString)

	r := client.NewRequest("GET", "/v1/sys/internal/ui/mounts/"+path)
	//nolint:staticcheck
	resp, err := client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return "", "", err
	}

	secret, err := api.ParseSecret(resp.Body)
	if err != nil {
		return "", "", err
	}

	if secret == nil {
		return "", "", errors.New("nil response from pre-flight request")
	}
	var mountPath string
	if mountPathRaw, ok := secret.Data["path"]; ok {
		mountPath = mountPathRaw.(string)
	}
	var pathType string
	if pathTypeRaw, ok := secret.Data["type"]; ok {
		pathType = pathTypeRaw.(string)
	}

	return mountPath, pathType, nil
}

// addPrefixToVKVPath copy paste from https://github.com/hashicorp/vault/blob/master/command/kv_helpers.go#L108
func addPrefixToVKVPath(p, mountPath, apiPrefix string) string {
	switch {
	case p == mountPath, p == strings.TrimSuffix(mountPath, "/"):
		return path.Join(mountPath, apiPrefix)
	default:
		p = strings.TrimPrefix(p, mountPath)
		return path.Join(mountPath, apiPrefix, p)
	}
}
