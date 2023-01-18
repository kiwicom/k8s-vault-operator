package main

import (
	"context"
	"flag"
	stdlog "log"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kiwicom/k8s-vault-operator/pkg/vault"
)

var (
	path   = flag.String("path", "", "path to VaultSecret k8s manifest")
	output = flag.String("o", "env", "output format: env/json/yaml")
)

func main() {
	flag.Parse()

	token := os.Getenv("VAULT_TOKEN")
	ctx := context.Background()

	appConfig, err := vault.NewAppConfig()
	if err != nil {
		stdlog.Fatal(err)
	}

	if len(token) == 0 {
		stdlog.Fatal("VAULT_TOKEN env variable must be set")
	}

	if len(*path) == 0 {
		stdlog.Fatal("path flag must be set")
	}

	secret, err := readYaml(*path)
	if err != nil {
		stdlog.Fatal(err)
	}

	authConfig := vault.NewAuthToken(token)

	vaultReader, err := vault.NewReader(authConfig, secret, ctrl.LoggerFrom(ctx), &appConfig)
	if err != nil {
		stdlog.Fatal(err)
	}

	err = vaultReader.ReadData()
	if err != nil {
		stdlog.Fatal(err)
	}

	err = vaultReader.WriteData(os.Stdout, *output)
	if err != nil {
		stdlog.Fatal(err)
	}
}
