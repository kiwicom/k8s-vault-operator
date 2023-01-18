# k8s-vault-operator

![Tests](https://github.com/kiwicom/k8s-vault-operator/actions/workflows/main.yml/badge.svg)

Syncing vault secrets to k8s secrets.

## Description
This repository contains the code and deployment manifests for a Kubernetes controller, which will automate the process of syncing Vault secrets into Kubernetes secrets.

## Operator configuration

There are several configuration options available to customize a deployment of an operator. They are set in 3 different places:

- default values are hardcoded into the operator itself
- remote Kustomize base of the operator contains a `ConfigMap` with explicit default values that can be generally used on all clusters
- cluster-specific Kustomize overrides in `ConfigMap` in infra cluster repository

Available configuration options:

- `OPERATOR_NAME`: a unique name for the operator - when running inside a cluster, it also serves as the name of the lock
- `LOG_LEVEL` (default `INFO`): specifies the ammount of logging output (values can be `INFO`, `DEBUG`)
- `VAULT_ADDR` (default `http://127.0.0.1:8200`): Vault address.
- `DEFAULT_SA_AUTH_PATH` (no default): this value has to be assigned per cluster, and it specifies the default Vault path used for SA/JWT authentication
    - by default, it should follow this convention: `auth/k8s/<cluster>/login`
- `DEFAULT_RECONCILE_PERIOD` (default `10m`): default reconcile period (i.e. how often will Vault secrets be synced)

Operator deployment injects environment variables from two Kubernetes Secrets:

- `system/vault-operator-env`: user-specified configuration (described above)

---

## How does the operator work?

The operator watches for changes on `VaultSecret` objects. Once a new `VaultSecret` is created or an existing is modified, the operator will receive a notification in its `reconcile loop`. Once inside the loop, it will:

- check validity of `VaultSecret`
- populate a list of Vault paths (expand recursive paths to a list of absolute paths)
- read values from all paths and look for overrides (in case of override, it will not sync anything)
- store combined values into a Kubernetes Secret in either JSON, ENV or YAML format
- schedule another iteration of the loop after `reconcilePeriod`


---

## VaultSecret manifests

Syncing is performed with a `VaultSecret` CRD (Custom Resource Definition).

Here you can see a minimum required example (with default values at bottom):

```yaml
apiVersion: k8s.kiwi.com/v1
kind: VaultSecret
metadata:
  name: test
  namespace: my-namespace
spec:
  paths:
    - path: kw/secret/infra/platform/my-cluster/my-namespace/recursive/path/*
    - path: kw/secret/infra/platform/my-cluster/my-namespace/my/sub/path/my-secret
      prefix: my_prefix

  # Those are defaults - you don't need to specify them!
  separator: "_"
  targetFormat: env
  reconcilePeriod: 10m
  targetSecretName: test # same as VaultSecret name
  addr: http://127.0.0.1:8200
  auth:
    serviceAccountRef:
      name: vault-operator-sync
      authPath: auth/kw/infra/platform/my-cluster/my-namespace/jerry/login
      role: my-namespace # same as namespace
```

Quick summary:

- `spec.addr`: Vault address used for authentication and fetching of Vault secrets
- `spec.separator`: this string is used as a separator/delimiter when outputting in env format
- `spec.paths.[].path`: a path to a Vault secret or a partial/recursive path to a Vault sub-path
- `spec.paths.[].prefix`: a prefix that will be applied to all values
- `spec.targetSecretName`: name of Kubernetes Secret where secrets will be synced into
- `spec.targetFormat`: output format of synced secrets
- `spec.reconcilePeriod`: amount of time between syncs
- `spec.auth.serviceAccountRef.name`: name of Service Account
- `spec.auth.serviceAccountRef.authPath`: Vault path used for Service Account authentication
- `spec.auth.serviceAccountRef.role`: Vault role used for Service Account authentication

All details about the `spec` are described in the following sections

### Service Account authentication

By default, a Kubernetes Namespace will contain a Service Account with permissions to access Vault paths relevant for that Namespace. Multiple `VaultSecret` manifests can re-use this Service Account. The name of this Service Account is the same as the name of the Namespace.

Full example:

```yaml
auth:
  serviceAccountRef:
    name: operator-test
    authPath: auth/kw/infra/platform/my-cluster/my-namespace/jerry/login
    role: operator-test
```

Minimum required:

```yaml
auth:
  serviceAccountRef:
    name: operator-test
```

Of the three values in `auth.serviceAccountRef`, only `name` is required and has to be set to the name of the Service Account used for Vault authentication. In most cases this will be the same as the name of Kubernetes Namespace. While this value could be infered, it is intentionally non-optional to force developers into thinking about Vault authentication.

- `authPath` will default to `DEFAULT_SA_AUTH_PATH` (operator config)
- `role` will default to the name of the Kubernetes Namespace

### Vault paths

The `VaultSecret` might have multiple paths defined. The values of paths are merged into one
kubernetes secret. If some path doesn't exist, it's skipped and vault operator create error log about this.
Also keys they're not matching naming convention (only `A-Z`, `a-z`, `0-9`, and `-_` for key name) are skipped
excluded from kubernetes secrets. If we have paths with same key name, the value of the secret
is overridden, but we don't ensure in which order.

There are two different kinds of paths you can specify:

- paths to Vault secrets
    - as absolute paths
    - as recursive paths

Several topics listed below will show examples of how the operator will combine secrets into their final output. They will reference the following simplified Vault structure:

- `secrets/frontend/config` contains `API_KEY=1` and `BACKEND_ENDPOINT=2`
- `secrets/backend/db/config` contains `USERNAME=3` and `PASSWORD=4`
- `secrets/backend/app/config` contains `WORKERS=5`

#### Absolute paths

```yaml
paths:
  - path: secrets/frontend/config
```

The most basic case, where you define an absolute path to a Vault secret.

This has to be a full path, the same you would use if you used the `vault` CLI tool:

```sh
vault kv get secrets/frontend/config
```

#### Recursive paths

```yaml
paths:
  - path: secrets/backend/*
```

Operator will recursively find all sub-paths and all secrets on those sub-paths. As with the absolute paths, the part before `/*` has to be a full path.

`*` can only be placed at the very end of a path - `secrets/*/config` is invalid.

Searching for `secrets/backend/*` will output (assuming `spec.separator` is `_` and `spec.targetFormat` is `env`):

```
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

Note that sub-paths and names of secrets are part of the output.

#### Prefixes

`spec.paths.path.prefix` gives you the ability to customize how different paths combine with each other in their final output. Prefixes can have `/` separators and should not be confused with `spec.separator`, which are used only when outputting. Where you put `/` separators in prefixes can have dramatic differences, especially in `json` output.

To illustrate how prefixes can be leveraged, we'll go over the same example using multiple different prefixes.

Base example, without prefixes:

```yaml
paths:
  - path: secrets/frontend/config
  - path: secrets/backend/*
```

```
API_KEY=1
BACKEND_ENDPOINT=2
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

```json
{
    "API_KEY": 1,
    "BACKEND_ENDPOINT": 2,
    "db": {
        "config": {
            "USERNAME": 3,
            "PASSWORD": 4
        }
    },
    "app": {
        "config": {
            "WORKERS": 5
        }
    }
}
```

Prefixing with `db_config`:

```yaml
paths:
  - path: secrets/frontend/config
    prefix: db_config
  - path: secrets/backend/*
```

Pay attention to the missing `_` between `db_config` and `API_KEY`.

```
db_configAPI_KEY=1
db_configBACKEND_ENDPOINT=2
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

```json
{
    "db_configAPI_KEY": 1,
    "db_configBACKEND_ENDPOINT": 2,
    "db": {
        "config": {
            "USERNAME": 3,
            "PASSWORD": 4
        }
    },
    "app": {
        "config": {
            "WORKERS": 5
        }
    }
}
```

Prefixing with `db_config/`:

```yaml
paths:
  - path: secrets/frontend/config
    prefix: db_config/
  - path: secrets/backend/*
```

```
db_config_API_KEY=1
db_config_BACKEND_ENDPOINT=2
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

```json
{
    "db_config": {
        "API_KEY": 1,
        "BACKEND_ENDPOINT": 2
    },
    "db": {
        "config": {
            "USERNAME": 3,
            "PASSWORD": 4
        }
    },
    "app": {
        "config": {
            "WORKERS": 5
        }
    }
}
```

Prefixing with `db/config`:

```yaml
paths:
  - path: secrets/frontend/config
    prefix: db/config
  - path: secrets/backend/*
```

Pay attention to the missing `_` between `db_config` and `API_KEY`.

```
db_configAPI_KEY=1
db_configBACKEND_ENDPOINT=2
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

```json
{
    "db": {
        "configAPI_KEY": 1,
        "configBACKEND_ENDPOINT": 2,
        "config": {
            "USERNAME": 3,
            "PASSWORD": 4
        }
    },
    "app": {
        "config": {
            "WORKERS": 5
        }
    }
}
```

Prefixing with `db/config/`:

```yaml
paths:
  - path: secrets/frontend/config
    prefix: db/config/
  - path: secrets/backend/*
```

```
db_config_API_KEY=1
db_config_BACKEND_ENDPOINT=2†
db_config_USERNAME=3
db_config_PASSWORD=4
app_config_WORKERS=5
```

```json
{
    "db": {
        "config": {
            "API_KEY": 1,
            "BACKEND_ENDPOINT": 2,
            "USERNAME": 3,
            "PASSWORD": 4
        }
    },
    "app": {
        "config": {
            "WORKERS": 5
        }
    }
}
```

### Saving to k8s secrets

`spec.targetSecretName` defines the name of Kubernetes Secret, where Vault secrets will be synced into. It will be created in the same Kubernetes Namespace where `VaultSecret` is.

#### Output formats

There are several different output formats and `spec.targetFormat` defines which one will be used.

You can use:

- `env`
- `json`
- `yaml`

### Reconcile period

`spec.reconcilePeriod` defines how often the operator will attempt to sync secrets. Default value is set to 10 minutes, which should be good for most cases.

**Note**: a fast reconcile period, along with a complex path structure, can cause a lot of requests to Vault. Keep this in mind when specifying this value.

### Adding VaultSecrets to Kustomize

Include the manifest in `kustomization.yaml` in your `overlay` as a `resource`:

```sh
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
...

resources:
  - name-of-your-vaultsync-manifest.yaml
```
## FAQ

### Why aren't my secrets syncing?

There are several checks made, to prevent syncing incomplete or invalid secrets.

- key in a Vault secret contains invalid characters (example: `a/b` is valid key in Vault, but cannot be used in Kubernetes)
- Vault path does not exist
- VaultSecret manifest is invalid (use [Reader tool](#reader-tool) to help with debugging)
- VaultSecret was re-applied too quickly (less than 10 seconds since last reconcile)
- wrongly configured Vault authentication (example: wrong `spec.auth.serviceAccountRef.name`)
- overrides have been detected (two keys from different Vault paths override each other, use [Reader tool](#reader-tool) to help with debugging)

We recommend to check vault operator logs and events with command:

```
kubectl get events -o json -n {desired_namespace} --field-selector involvedObject.kind=VaultSecret | jq '.items[].message'
```

### Where are we going to see it when does it change the secret?

It updates the status of `VaultSecret` resource with `LastUpdate` field and adds an `event` to `VaultSecret`. You can see both status and events with `kubectl describe vaultsecret test`.

It is also part of operator logs, there will be message like:

> Secret exists, data not equal, updating: gds-queue-handler Secret.Name: gds-queue-handler-secrets

---

## Reader tool

The `reader` tool has been developed to help create and debug `VaultSecret` manifests. It will parse a manifest, connect to Vault and output the results to `stdout`. Use this tool to configure your `paths` and `prefixes` before deploying `VaultSecret` manifests.

### Authentication

`reader` does not authenticate using Service Account, instead it uses Token authentication.

To use `reader`, you must provide `VAULT_ADDR` and `VAULT_TOKEN` environment variables.

```sh
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=mysecrettoken
```

**Note**: your token will require permissions to access the paths specified in `VaultSecret` you are testing.

### Flags

- `-path`: path to `VaultSecret` manifest you are testing
- `-state`: path to state file, file does not have to exist on first run
- `-o`: output format, defaults to `env`

---

## Debugging the operator

To view the operator logs, connect to your cluster, run `kubectl get pod -n system` and look for:

```sh
...
vault-operator-v1-<RANDOM_ID>     1/1     Running   0          3h4m
...
```

Next, run `kubectl logs -f vault-operator-v1-<RANDOM_ID> -n system`.

You will see output of every `Secret` and `VaultSecret` which passes through the operator. If the operator performs an update, you will see output like this:
```json lines
{"level":"info","ts":"2023-01-16T14:02:00+01:00","msg":"Reconciling VaultSecret","controller":"vaultsecret","controllerGroup":"k8s.kiwi.com","controllerKind":"VaultSecret","VaultSecret":{"name":"vaultsecret-sample","namespace":"default"},"namespace":"default","name":"vaultsecret-sample","reconcileID":"f0780ec1-d164-4699-96c9-4d9e3e7befd7"}
{"level":"info","ts":"2023-01-16T14:02:00+01:00","msg":"Secret exists, data not equal, updating: default Secret.name: secrets-from-vault","controller":"vaultsecret","controllerGroup":"k8s.kiwi.com","controllerKind":"VaultSecret","VaultSecret":{"name":"vaultsecret-sample","namespace":"default"},"namespace":"default","name":"vaultsecret-sample","reconcileID":"f0780ec1-d164-4699-96c9-4d9e3e7befd7"}
{"level":"info","ts":"2023-01-16T14:02:00+01:00","msg":"Finished reconciling VaultSecret","controller":"vaultsecret","controllerGroup":"k8s.kiwi.com","controllerKind":"VaultSecret","VaultSecret":{"name":"vaultsecret-sample","namespace":"default"},"namespace":"default","name":"vaultsecret-sample","reconcileID":"f0780ec1-d164-4699-96c9-4d9e3e7befd7"}
```
### Events

Operator will save errors and certain key checkpoints as `events` to `VaultSecret` it currently syncs. To view them, run `kubectl describe vaultsecret myvaultsecret -n mynamespace`. Multiple events of the same type will be merged together in this output.

Old events will be removed after some time, customizable per cluster and on GKE this is set to 1 hour.

---
## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make docker-build docker-push IMG=<some-registry>/k8s-vault-operator:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/k8s-vault-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster 

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

### Local development

1. Choose your kube context
2. Run `make install` - this will install CRDs into your K8s cluster
3. Spin up vault server - eg. `docker-compose up vault`
4. Run `make run` - this will spin up operator on your machine
    - alternative `make deploy` - deploy controller to your cluster
5. Sample vault secret manifest is in `config/samples/v1_vaultsecret.yaml`
    - `kubectl apply -f config/samples/v1_vaultsecret.yaml`

### Testing
Tests are written in form of cases. Each case consists of `vault_secret.yaml` and expected results:
 - `expected.env`
 - `expected.json`
 - `expected.error`

In `vault_secret.yaml` you need to specify only `paths` inside `spec`. `name`, `namespace`, `token` is overridden by test itself.

For each target format (env, json) VaultSecret manifest is created.

Tests are using test-env for k8s cluster and Vault server itself. So in order to run tests you need run Vault server:
 - Using `docker-compose up vault`
 - Local vault server
   - `export VAULT_DEV_ROOT_TOKEN_ID=testtoken`
   - `vault server -dev -dev-listen-address=0.0.0.0:8200`

Afterwards just run `make test`.
## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
