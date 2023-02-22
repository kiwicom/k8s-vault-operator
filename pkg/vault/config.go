package vault

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	vaultAPI "github.com/hashicorp/vault/api"
	"github.com/knadh/koanf"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
)

var k = koanf.New(".")

type AppConfig struct {
	LogLevel                string        `koanf:"log_level"`
	ClientTimeout           time.Duration `koanf:"client_timeout"`
	ClientMaxRetries        int           `koanf:"client_max_retries"`
	DefaultSAAuthPath       string        `koanf:"default_sa_auth_path"`
	DefaultSAName           string        `koanf:"default_sa_name"`
	DefaultReconcilePeriod  string        `koanf:"default_reconcile_period"`
	OperatorRole            string        `koanf:"operator_role"`
	Role                    string        `koanf:"role"`
	DefaultVaultAddr        string        `koanf:"vault_addr"`
	MaxConcurrentReconciles int           `koanf:"max_concurrent_reconciles"`
}

func NewAppConfig() (AppConfig, error) {
	var cfg AppConfig
	err := k.Load(confmap.Provider(map[string]any{
		"log_level":                 "INFO",
		"default_sa_auth_path":      "",
		"default_sa_name":           "vault-operator-sync",
		"default_reconcile_period":  "10m",
		"operator_role":             "vault-operator",
		"vault_addr":                "http://127.0.0.1:8200",
		"max_concurrent_reconciles": 5,
	}, "."), nil)
	if err != nil {
		return cfg, fmt.Errorf("default setting load: %w", err)
	}

	err = k.Load(env.Provider("", ".", strings.ToLower), nil)
	if err != nil {
		return cfg, fmt.Errorf("env setting load: %w", err)
	}

	if err := k.Unmarshal("", &cfg); err != nil {
		return cfg, fmt.Errorf("unmarshal: %w", err)
	}

	return cfg, nil
}

func NewClient(cfg AppConfig) (*vaultAPI.Client, error) {
	operatorClient, err := vaultAPI.NewClient(&vaultAPI.Config{
		Address:    cfg.DefaultVaultAddr,
		MaxRetries: cfg.ClientMaxRetries,
		Timeout:    cfg.ClientTimeout,
		Backoff:    retryablehttp.LinearJitterBackoff,
	})
	if err != nil {
		return nil, fmt.Errorf("could not initialize state vault client: %w", err)
	}

	return operatorClient, nil
}
