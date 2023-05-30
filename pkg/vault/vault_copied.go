// nolint
package vault

import (
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/hashicorp/vault/api"
)

// copied from https://github.com/hashicorp/vault/blob/main/command/kv_helpers.go#L44
func kvPreflightVersionRequest(client *api.Client, path string) (string, int, error) {
	// We don't want to use a wrapping call here so save any custom value and
	// restore after
	currentWrappingLookupFunc := client.CurrentWrappingLookupFunc()
	client.SetWrappingLookupFunc(nil)
	defer client.SetWrappingLookupFunc(currentWrappingLookupFunc)
	currentOutputCurlString := client.OutputCurlString()
	client.SetOutputCurlString(false)
	defer client.SetOutputCurlString(currentOutputCurlString)
	currentOutputPolicy := client.OutputPolicy()
	client.SetOutputPolicy(false)
	defer client.SetOutputPolicy(currentOutputPolicy)

	r := client.NewRequest("GET", "/v1/sys/internal/ui/mounts/"+path)
	resp, err := client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		// If we get a 404 we are using an older version of vault, default to
		// version 1
		if resp != nil {
			if resp.StatusCode == 404 {
				return "", 1, nil
			}

			// if the original request had the -output-curl-string or -output-policy flag,
			if (currentOutputCurlString || currentOutputPolicy) && resp.StatusCode == 403 {
				// we provide a more helpful error for the user,
				// who may not understand why the flag isn't working.
				err = fmt.Errorf(
					`This output flag requires the success of a preflight request 
to determine the version of a KV secrets engine. Please 
re-run this command with a token with read access to %s`, path)
			}
		}

		return "", 0, err
	}

	secret, err := api.ParseSecret(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if secret == nil {
		return "", 0, errors.New("nil response from pre-flight request")
	}
	var mountPath string
	if mountPathRaw, ok := secret.Data["path"]; ok {
		mountPath = mountPathRaw.(string)
	}
	options := secret.Data["options"]
	if options == nil {
		return mountPath, 1, nil
	}
	versionRaw := options.(map[string]interface{})["version"]
	if versionRaw == nil {
		return mountPath, 1, nil
	}
	version := versionRaw.(string)
	switch version {
	case "", "1":
		return mountPath, 1, nil
	case "2":
		return mountPath, 2, nil
	}

	return mountPath, 1, nil
}

// addPrefixToKVPath copy and paste from https://github.com/hashicorp/vault/blob/main/command/kv_helpers.go#L124
func addPrefixToKVPath(p, mountPath, apiPrefix string) string {
	if p == mountPath || p == strings.TrimSuffix(mountPath, "/") {
		return path.Join(mountPath, apiPrefix)
	}

	tp := strings.TrimPrefix(p, mountPath)
	for {
		// If the entire mountPath is included in the path, we are done
		if tp != p {
			break
		}
		// Trim the parts of the mountPath that are not included in the
		// path, for example, in cases where the mountPath contains
		// namespaces which are not included in the path.
		partialMountPath := strings.SplitN(mountPath, "/", 2)
		if len(partialMountPath) <= 1 || partialMountPath[1] == "" {
			break
		}
		mountPath = strings.TrimSuffix(partialMountPath[1], "/")
		tp = strings.TrimPrefix(tp, mountPath)
	}

	return path.Join(mountPath, apiPrefix, tp)
}

// copied from https://github.com/hashicorp/vault/blob/main/command/kv_helpers.go#L15
func kvReadRequest(client *api.Client, path string, params map[string]string) (*api.Secret, error) {
	r := client.NewRequest("GET", "/v1/"+path)
	for k, v := range params {
		r.Params.Set(k, v)
	}
	resp, err := client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if resp != nil && resp.StatusCode == 404 {
		secret, parseErr := api.ParseSecret(resp.Body)
		switch parseErr {
		case nil:
		case io.EOF:
			return nil, ErrNotFound
		default:
			return nil, err
		}
		if secret != nil && (len(secret.Warnings) > 0 || len(secret.Data) > 0) {
			return secret, nil
		}
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return api.ParseSecret(resp.Body)
}
