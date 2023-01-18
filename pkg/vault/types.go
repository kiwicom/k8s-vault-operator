package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jeremywohl/flatten"
	"gopkg.in/yaml.v3"
)

const (
	TypeJSON = "json"
	TypeEnv  = "env"
	TypeYaml = "yaml"
)

type Secrets map[string]any

type PathData struct {
	BasePath string             `json:"base_path"`
	Prefix   string             `json:"prefix"`
	Paths    map[string]Secrets `json:"paths"`
}

func (pd *PathData) GetRelativePath(path string) string {
	return strings.ReplaceAll(path, pd.BasePath, "")
}

// Data is a map of any, because the value can be either Data, Secrets or a string
type Data map[string]any

func (d Data) AddNode(name string) (Data, error) {
	// check if name already exists
	if node, ok := d[name]; ok {
		switch n := node.(type) {
		case Data:
			return n, nil
		case string:
			// FIXME: show full path in error
			return nil, fmt.Errorf("override detected: key %q is already used", name)
		default:
			return nil, fmt.Errorf("invalid type received, got: %T", node)
		}
	}

	newNode := make(Data)
	d[name] = newNode

	return newNode, nil
}

// AddSecrets is used to add multiple key=value pairs to Data
func (d Data) AddSecrets(secrets Secrets) error {
	for key, val := range secrets {
		// check if key already exists
		if _, ok := d[key]; ok {
			// FIXME: add a method to Data that calculates full path to here
			return fmt.Errorf("override detected: key %q is already used", key)
		}

		d[key] = val
	}

	return nil
}

func (d Data) JSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

func (d Data) Yaml() ([]byte, error) {
	return yaml.Marshal(d)
}

func (d Data) ENV(separator string) (map[string]any, error) {
	return d.createENV(separator)
}

func (d Data) ENVString(separator string) ([]byte, error) {
	envs, err := d.ENV(separator)

	if err != nil {
		return nil, err
	}

	tmp := make([]string, 0, len(envs))
	for k, v := range envs {
		tmp = append(tmp, fmt.Sprintf("%s=%v\n", k, v))
	}

	sort.Strings(tmp)

	var b bytes.Buffer
	for _, v := range tmp {
		b.WriteString(v)
	}

	return b.Bytes(), nil
}

func (d Data) createENV(separator string) (map[string]any, error) {
	b, err := d.JSON()

	if err != nil {
		return nil, err
	}

	style := flatten.SeparatorStyle{Middle: separator}
	fs, err := flatten.FlattenString(string(b), "", style)

	if err != nil {
		return nil, fmt.Errorf("failed to flatten data: %w", err)
	}

	data := make(map[string]any)
	if err := json.Unmarshal([]byte(fs), &data); err != nil {
		return nil, err
	}

	return data, nil
}
