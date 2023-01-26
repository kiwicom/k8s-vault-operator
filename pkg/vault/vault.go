package vault

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-retryablehttp"
	vaultApi "github.com/hashicorp/vault/api"

	v1 "github.com/kiwicom/k8s-vault-operator/api/v1"
)

type Reader struct {
	client *vaultApi.Client
	secret *v1.VaultSecret
	paths  []PathData
	data   Data
	cfg    *AppConfig
	log    logr.Logger
}

func NewReader(tokener Tokener, secret *v1.VaultSecret, logger logr.Logger, cfg *AppConfig) (*Reader, error) {
	client, err := vaultApi.NewClient(&vaultApi.Config{
		Address:    secret.Spec.Addr,
		MaxRetries: cfg.ClientMaxRetries,
		Timeout:    cfg.ClientTimeout,
		Backoff:    retryablehttp.LinearJitterBackoff,
	})

	if err != nil {
		return nil, fmt.Errorf("could not create Vault client: %w", err)
	}

	r := Reader{
		client: client,
		secret: secret,
		cfg:    cfg,
		log:    logger,
	}

	token, err := tokener.Token()
	if err != nil {
		return nil, fmt.Errorf("retrieve token: %w", err)
	}
	client.SetToken(token)

	return &r, nil
}

func (r *Reader) GetData() Data {
	return r.data
}

func (r *Reader) ReadData() error {
	if err := r.getAbsolutePaths(); err != nil {
		return fmt.Errorf("failed to get paths from vault: %w", err)
	}

	if err := r.readSecretsFromPaths(); err != nil {
		return fmt.Errorf("failed to read paths from vault: %w", err)
	}

	if err := r.createVaultData(); err != nil {
		return fmt.Errorf("failed to create vault data: %w", err)
	}

	return nil
}

// WriteData takes an io.Reader and writes bytes in the specified output format
func (r *Reader) WriteData(w io.Writer, format string) error {
	var (
		b   []byte
		err error
	)

	switch strings.ToLower(format) {
	case TypeJSON:
		b, err = r.data.JSON()
		b = append(b, byte('\n'))
	case TypeEnv:
		b, err = r.data.ENVString(r.secret.Spec.GetSeparator())
	case TypeYaml:
		b, err = r.data.Yaml()
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}

	if err != nil {
		return fmt.Errorf("marshall failed: %w", err)
	}

	_, err = w.Write(b)
	if err != nil {
		return fmt.Errorf("could not write: %w", err)
	}

	return nil
}

// getAbsolutePaths populates []PathData with absolute paths to Vault secrets.
// In the case of "secret/recursive/path/*", it will recursively call Vault and
// find all child Secrets with their absolute paths.
func (r *Reader) getAbsolutePaths() error {
	for _, path := range r.secret.Spec.Paths {
		paths := make(map[string]Secrets)
		var cleanedPath string

		// remove leading /, to avoid unnecessary confusion with 403 errors
		// because "my/path" and "/my/path" are not the same
		if path.Path[0] == '/' {
			path.Path = path.Path[1:]
		}

		// if last char is "*", then recursively call Vault until all subPaths are found
		if path.Path[len(path.Path)-1] == '*' {
			// remove "*" before calling Vault
			cleanedPath = path.Path[0 : len(path.Path)-1]

			subPaths, err := r.getPathsRecursive(cleanedPath)
			if err != nil {
				return err
			}

			for _, subPath := range subPaths {
				paths[subPath] = make(Secrets)
			}
		} else {
			// non-recursive path
			cleanedPath = path.Path
			paths[path.Path] = make(Secrets)
		}

		r.paths = append(r.paths, PathData{
			BasePath: cleanedPath,
			Prefix:   path.Prefix,
			Paths:    paths,
		})
	}

	return nil
}

// Function iterate over reader's paths, merge secrets into one piece.
// If some path doesn't exist, it's skipped and function finish with
// no error. It's more reliable than let function crash the
// reconciliation loop.
//
// This function don't take list of paths and don't return the secrets.
// Instead of this, the function works with reader's state. Don't know the
// exact reason. Anyway the secrets are filled into reader's stateReader.
func (r *Reader) readSecretsFromPaths() error {
	reconcilePeriod, err := time.ParseDuration(r.secret.Spec.ReconcilePeriod)
	if err != nil {
		return fmt.Errorf("failed to parse reconcile period %q: %w", r.secret.Spec.ReconcilePeriod, err)
	}

	pathReader := PathReader{
		Client:          r.client,
		log:             r.log,
		reconcilePeriod: reconcilePeriod,
	}

	for _, pathData := range r.paths {
		for absolutePath, secrets := range pathData.Paths {
			secretsData, err := pathReader.Read(absolutePath)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					// make a log entry and skip the broken path
					r.log.Error(err, absolutePath)
					continue
				}
				return err
			}

			// the keys might contain only letter, numbers and -,_ characters
			// otherwise the key is ignored
			for k, v := range secretsData {
				if ok := r.validateKey(k); !ok {
					r.log.Error(fmt.Errorf("invalid key: %v", k), k)
					continue
				}
				secrets[k] = v
			}
		}
	}

	return nil
}

func (r *Reader) getPathsRecursive(path string) ([]string, error) {
	mountPath, version, err := kvPreflightVersionRequest(r.client, path)

	if err != nil {
		return nil, err
	}

	if version != 1 && version != 2 {
		return nil, fmt.Errorf("unsupported engine for recursion, expected 1 or 2, got %d", version)
	}
	apiPath := path
	if version == 2 {
		apiPath = addPrefixToKVPath(path, mountPath, "metadata")
	}

	secretValues, err := r.client.Logical().List(apiPath)
	if err != nil {
		return nil, fmt.Errorf("could not read Vault path %q: %w", apiPath, err)
	}

	if secretValues == nil {
		return nil, fmt.Errorf("no value found in path: %q", path)
	}

	keys, ok := secretValues.Data["keys"].([]any)
	if !ok {
		return nil, fmt.Errorf("cannot cast keys to slice of interfaces at path: %q", apiPath)
	}

	var paths []string
	for _, k := range keys {
		key, ok := k.(string)
		if !ok {
			return nil, fmt.Errorf("cannot cast keys to string at path: %q", apiPath)
		}

		fullPath := path + key

		// check if current key is a directory
		if key[len(key)-1] == '/' {
			// and fetch all subPaths
			subPaths, err := r.getPathsRecursive(fullPath)
			if err != nil {
				return nil, err
			}

			paths = append(paths, subPaths...)
		} else {
			// else, just add it to the list
			paths = append(paths, fullPath)
		}
	}

	return paths, nil
}

func (r *Reader) createVaultData() error {
	rootNode := make(Data)

	for _, pathData := range r.paths {
		currentNode := rootNode

		for path, secrets := range pathData.Paths {
			// get relative path (abs path - base path) with prefix
			relativePath := pathData.GetRelativePath(path)
			relativePathWithPrefix := pathData.Prefix + relativePath
			secretKeyPrefix := ""

			// if length is 0, it means we have actual secrets
			if len(relativePathWithPrefix) == 0 {
				if err := currentNode.AddSecrets(secrets); err != nil {
					return err
				}

				continue
			}

			// each path segment has to become a separate node
			pathSegments := strings.Split(relativePathWithPrefix, "/")

			// in this case, we have a concrete path to a secret with a prefix
			// because we want to avoid adding a separator between the last element of prefix and
			// the key of a secret value, it has to be treated differently
			if relativePath == "" && pathData.Prefix != "" {
				secretKeyPrefix = pathSegments[len(pathSegments)-1]
				pathSegments = pathSegments[:len(pathSegments)-1]
			}

			// previousNode always points to the parent node
			previousNode := currentNode

			for _, pathSegment := range pathSegments {
				var err error

				previousNode, err = previousNode.AddNode(pathSegment)
				if err != nil {
					return err
				}
			}

			newSecrets := make(Secrets, len(secrets))

			for k, v := range secrets {
				newSecrets[secretKeyPrefix+k] = v
			}

			if err := previousNode.AddSecrets(newSecrets); err != nil {
				return err
			}
		}
	}

	r.data = rootNode

	return nil
}

func (r *Reader) validateKey(key string) bool {
	for _, r := range key {
		// The keys of data  must consist of alphanumeric characters, -, _ or .
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '.' && r != '_' {
			return false
		}
	}
	return true
}
