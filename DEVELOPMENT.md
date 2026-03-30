# Development Guide

This guide covers everything you need to get started contributing to the AxonOps Operator. It focuses on local setup, day-to-day development tasks, and the conventions the project follows. For the contribution workflow (branches, commits, PRs) see [CONTRIBUTING.md](CONTRIBUTING.md).

## Table of Contents

- [Prerequisites](#prerequisites)
- [Dev Container (Recommended)](#dev-container-recommended)
- [Manual Setup](#manual-setup)
- [Project Structure](#project-structure)
- [Daily Development Workflow](#daily-development-workflow)
- [Code Generation](#code-generation)
- [Testing](#testing)
- [Running the Operator Locally](#running-the-operator-locally)
- [Deploying to a Cluster](#deploying-to-a-cluster)
- [Adding New CRDs or Controllers](#adding-new-crds-or-controllers)
- [Code Conventions](#code-conventions)
- [Auto-Generated Files — Never Edit These](#auto-generated-files--never-edit-these)
- [Debugging](#debugging)
- [References](#references)

---

## Prerequisites

| Tool | Version | Notes |
|---|---|---|
| Go | 1.25.3+ | Version in `go.mod` is authoritative |
| Docker | Any recent | Required for building images and running Kind |
| Kind | Latest | Required for E2E tests only |
| kubectl | 1.28+ | Points at a cluster or Kind node |
| kubebuilder | 4.13.0+ | Required only when adding new CRDs or webhooks |
| cert-manager | 1.20.0+ | Required for testing internal database components |

Make targets download their own toolchain binaries into `./bin/` (`kustomize`, `controller-gen`, `setup-envtest`, `golangci-lint`). You do not need to install these globally.

---

## Dev Container (Recommended)

The repository ships a VS Code dev container that pre-installs all tools. Open the project in VS Code, accept the prompt to reopen in the container, and the post-install script installs Go, kubectl, Kind, kubebuilder, and Docker-in-Docker automatically.

The container is defined in `.devcontainer/devcontainer.json`. The setup script is `.devcontainer/post-install.sh`.

---

## Manual Setup

```bash
git clone https://github.com/axonops/axonops-operator.git
cd axonops-operator

# Download Go module dependencies
go mod download

# Download build toolchain into ./bin/
make controller-gen kustomize setup-envtest golangci-lint

# Verify everything compiles
make build
```

---

## Project Structure

This is a **multi-group kubebuilder project** (`multigroup: true` in `PROJECT`). APIs and controllers are organised by API group.

```
axonops-operator/
├── api/
│   ├── v1alpha1/                   # core.axonops.com group
│   │   ├── axonopsplatform_types.go
│   │   ├── axonopsconnection_types.go
│   │   ├── groupversion_info.go
│   │   └── zz_generated.deepcopy.go   # AUTO-GENERATED — do not edit
│   └── alerts/
│       └── v1alpha1/               # alerts.axonops.com group
│           ├── axonopsmetricalert_types.go
│           ├── axonopslogalert_types.go
│           ├── axonopsalertroute_types.go
│           ├── axonopshealthcheckhttp_types.go
│           ├── axonopshealthcheckshell_types.go
│           ├── axonopshealthchecktcp_types.go
│           └── zz_generated.deepcopy.go   # AUTO-GENERATED — do not edit
├── internal/
│   ├── controller/
│   │   ├── axonopsplatform_controller.go    # Main AxonOpsPlatform reconciler
│   │   ├── axonopsplatform_controller_test.go
│   │   ├── suite_test.go
│   │   ├── alerts/                        # Alert CRD reconcilers
│   │   │   ├── axonopsmetricalert_controller.go
│   │   │   ├── axonopslogalert_controller.go
│   │   │   ├── axonopsalertroute_controller.go
│   │   │   ├── axonopshealthcheckhttp_controller.go
│   │   │   ├── axonopshealthcheckshell_controller.go
│   │   │   ├── axonopshealthchecktcp_controller.go
│   │   │   └── suite_test.go
│   │   ├── backups/                       # Backup CRD reconcilers
│   │   ├── kafka/                         # Kafka CRD reconcilers
│   │   └── common/
│   │       └── connection.go              # Shared AxonOpsConnection resolver
│   └── axonops/
│       ├── client.go                      # AxonOps REST API client
│       └── types.go                       # API request/response types
├── config/
│   ├── crd/bases/                  # AUTO-GENERATED CRDs — do not edit
│   ├── rbac/role.yaml              # AUTO-GENERATED RBAC — do not edit
│   └── samples/                   # Example CRs (edit these freely)
├── test/
│   ├── bdd/                       # Gherkin feature files (acceptance criteria)
│   └── e2e/                       # End-to-end tests (requires Kind)
├── examples/
│   ├── axonops/                   # Full AxonOpsPlatform examples
│   └── k8ssandra/                 # K8ssandra-specific examples
├── charts/axonops-operator/       # Helm chart (published to GHCR)
├── cmd/main.go                    # Manager entry point
├── Makefile
└── PROJECT                        # Kubebuilder metadata — do not edit
```

### API Groups and CRDs

**`core.axonops.com`** (`api/v1alpha1/`)

| Kind | Purpose |
|---|---|
| `AxonOpsPlatform` | Deploys and manages axon-server, axon-dash, axondb-timeseries, axondb-search |
| `AxonOpsConnection` | Holds reusable AxonOps API credentials referenced by alert CRDs |

**`alerts.axonops.com`** (`api/alerts/v1alpha1/`)

| Kind | Purpose |
|---|---|
| `AxonOpsMetricAlert` | Metric threshold alert rules |
| `AxonOpsLogAlert` | Log pattern alert rules |
| `AxonOpsAlertRoute` | Alert routing and notification channel configuration |
| `AxonOpsAlertEndpoint` | Integration endpoint definitions (PagerDuty, Slack, etc.) |
| `AxonOpsHealthcheckHTTP` | HTTP endpoint healthchecks |
| `AxonOpsHealthcheckShell` | Shell script healthchecks |
| `AxonOpsHealthcheckTCP` | TCP port healthchecks |
| `AxonOpsDashboardTemplate` | Declarative dashboard management |
| `AxonOpsAdaptiveRepair` | Adaptive Cassandra repair scheduling |
| `AxonOpsScheduledRepair` | Cron-based Cassandra scheduled repairs |
| `AxonOpsCommitlogArchive` | Commitlog archive settings |
| `AxonOpsSilenceWindow` | Alert silence windows |
| `AxonOpsLogCollector` | Log collector configuration |

**`backups.axonops.com`** (`api/backups/v1alpha1/`)

| Kind | Purpose |
|---|---|
| `AxonOpsBackup` | Cassandra scheduled snapshot backups (S3/SFTP/Azure) |

**`kafka.axonops.com`** (`api/kafka/v1alpha1/`)

| Kind | Purpose |
|---|---|
| `AxonOpsKafkaTopic` | Kafka topic lifecycle management |
| `AxonOpsKafkaACL` | Kafka ACL entry management |
| `AxonOpsKafkaConnector` | Kafka Connect connector management |

---

## Daily Development Workflow

### After editing `*_types.go` or kubebuilder markers

```bash
make manifests   # Regenerate CRDs and RBAC from markers
make generate    # Regenerate DeepCopy methods
```

Both commands must run before committing. The CI pipeline runs them too and will fail if generated files are stale.

### After editing any `*.go` file

```bash
make lint-fix    # Auto-fix code style issues
make test        # Run unit tests
```

### Full quality check before opening a PR

```bash
make fmt && make vet && make lint && make test
```

All four must pass with no errors or warnings.

---

## Code Generation

The project relies on two code-generation steps driven by kubebuilder markers in `*_types.go` and `*_controller.go` files.

### `make manifests`

Reads `// +kubebuilder:...` markers and writes:
- `config/crd/bases/*.yaml` — CustomResourceDefinition YAML files
- `config/rbac/role.yaml` — ClusterRole for the manager
- `config/webhook/manifests.yaml` — Webhook configurations (when webhooks are added)

### `make generate`

Runs `controller-gen object` to write `zz_generated.deepcopy.go` in every `api/` package.

### Common markers reference

```go
// On the type:
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"

// On fields:
// +kubebuilder:validation:Required
// +kubebuilder:validation:Optional
// +kubebuilder:validation:Minimum=1
// +kubebuilder:validation:MaxLength=253
// +kubebuilder:validation:Pattern="^[a-z][a-z0-9-]*$"
// +kubebuilder:validation:Enum=cassandra;dse;scylla
// +kubebuilder:default="somevalue"
```

Use `metav1.Condition` for status conditions and `metav1.Time` for timestamps — not plain strings.

---

## Testing

### Unit tests (envtest)

```bash
make test
```

Unit tests use [envtest](https://book.kubebuilder.io/reference/envtest.html), which runs a real Kubernetes API server and etcd binary. No cluster is required. `make test` downloads the correct binaries via `setup-envtest` and sets `KUBEBUILDER_ASSETS` automatically.

Tests are written with **Ginkgo v2 + Gomega**. See `internal/controller/suite_test.go` and `internal/controller/alerts/suite_test.go` for test suite setup.

**Important cert-manager note**: cert-manager CRDs are not available in the envtest environment. Tests that exercise the `AxonOpsPlatform` controller with internal database components (TimeSeries or Search StatefulSets) will encounter the cert-manager `ClusterIssuer` step. Tests that use external database configuration bypass this step entirely. When writing new controller tests, use the external database path if you do not need to test cert-manager integration specifically.

```bash
# Run tests for a specific package
go test ./internal/controller/... -v

# Run a single test by name
go test ./internal/controller/... -run "TestAxonOpsPlatformController"

# View coverage report
make test
go tool cover -html=cover.out
```

### BDD feature files

Acceptance criteria are specified in Gherkin format under `test/bdd/`. Every significant feature or bug fix should have a corresponding `.feature` file committed to the branch **before** implementation begins.

```
test/bdd/
├── internal-deployment.feature
├── authentication-lifecycle.feature
├── component-lifecycle.feature
├── external-timeseries.feature
├── external-searchdb.feature
├── cert-manager-integration.feature
├── alert-metric-alert.feature
├── alert-log-alert.feature
├── alert-route.feature
├── healthcheck-http.feature
├── healthcheck-tcp.feature
├── healthcheck-shell.feature
└── ...
```

Step definitions are implemented separately (Godog or similar). Feature files serve as living documentation of expected behaviour even before step definitions exist.

### End-to-end tests (Kind)

E2E tests require a real Kubernetes cluster via Kind. The Makefile creates and tears down the cluster automatically.

```bash
# Create the Kind cluster, run e2e tests, then delete the cluster
make test-e2e

# Manage the cluster manually
make setup-test-e2e       # Create cluster only
kind get clusters         # Verify cluster exists
make cleanup-test-e2e     # Delete cluster
```

The default Kind cluster name is `axonops-operator-test-e2e` (set via `KIND_CLUSTER` in the Makefile). Do not run E2E tests against a development or production cluster.

---

## Running the Operator Locally

Running locally uses your current kubeconfig context. Install the CRDs first, then start the manager process on your machine.

```bash
# 1. Install CRDs into the cluster in your current kubeconfig context
make install

# 2. Run the manager locally (Ctrl+C to stop)
make run
```

The manager connects to the cluster but runs as a local process — useful for fast iteration with a debugger or `fmt.Println` tracing.

To apply the sample CRs:

```bash
kubectl apply -k config/samples/
```

For more complete examples see `examples/axonops/` (quickstart, medium, and complex configurations) and `examples/k8ssandra/` (K8ssandra-specific setup).

---

## Deploying to a Cluster

```bash
# Build and push the image
export IMG=myregistry/axonops-operator:dev
make docker-build docker-push IMG=$IMG

# Or load into Kind without a registry
kind load docker-image $IMG --name axonops-operator-test-e2e

# Deploy the manager and CRDs
make deploy IMG=$IMG

# Check the manager is running
kubectl logs -n axonops-operator-system deployment/axonops-operator-controller-manager -c manager -f

# Remove the deployment when done
make undeploy

# Uninstall CRDs
make uninstall
```

To build a single `dist/install.yaml` bundle for distribution:

```bash
make build-installer IMG=$IMG
```

---

## Adding New CRDs or Controllers

Always use the kubebuilder CLI to scaffold new types. Do not create files manually — kubebuilder wires the type into `cmd/main.go` and updates `PROJECT` automatically.

### Add a new CRD in an existing group

```bash
# core.axonops.com group
kubebuilder create api --group core --version v1alpha1 --kind MyNewKind

# alerts.axonops.com group
kubebuilder create api --group alerts --version v1alpha1 --kind MyNewAlert
```

### Add a webhook

```bash
kubebuilder create webhook --group core --version v1alpha1 --kind MyNewKind \
  --defaulting --programmatic-validation
```

After scaffolding:

```bash
make manifests generate   # Regenerate CRDs, RBAC, and DeepCopy
```

### Scaffold a controller for an external type

```bash
kubebuilder create api \
  --group cert-manager --version v1 --kind Certificate \
  --controller=true --resource=false \
  --external-api-path=github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1 \
  --external-api-domain=io \
  --external-api-module=github.com/cert-manager/cert-manager
```

---

## Code Conventions

### Controller implementation rules

**Idempotent reconciliation** — The `Reconcile` function must be safe to call multiple times with the same inputs and produce the same result. Never assume state from a previous reconciliation is still in place.

**Re-fetch before updates** — Always call `r.Get(ctx, req.NamespacedName, obj)` immediately before `r.Status().Update()` or `r.Update()`. Stale resource versions cause update conflicts.

```go
if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
    return ctrl.Result{}, client.IgnoreNotFound(err)
}
```

**Owner references** — Set `SetControllerReference` on all secondary resources so they are garbage-collected automatically when the parent CR is deleted.

```go
if err := controllerutil.SetControllerReference(owner, child, r.Scheme); err != nil {
    return ctrl.Result{}, err
}
```

**Watch secondary resources** — Use `.Owns()` or `.Watches()` in the controller's `SetupWithManager` so changes to owned resources trigger reconciliation. Do not rely on `RequeueAfter` alone.

**Status conditions** — Use `metav1.Condition` and `meta.SetStatusCondition`. The standard `Ready` condition communicates overall health to users and tooling.

```go
meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    ObservedGeneration: obj.Generation,
    Reason:             "Synced",
    Message:            "Resource synced successfully",
})
```

**Finalizers** — Add a finalizer when the controller manages external resources (AxonOps API objects, cloud resources). Remove it only after successful cleanup.

### Logging conventions

Follow the [Kubernetes logging message style guidelines](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines):

- Start with a capital letter
- Do not end with a period
- Use past tense for failures: "Could not delete Pod" not "Cannot delete Pod"
- Always name the object type: "Deleted StatefulSet" not "Deleted"
- Use balanced key-value pairs for context

```go
log := log.FromContext(ctx)
log.Info("Starting reconciliation")
log.Info("Created StatefulSet", "name", sts.Name, "namespace", sts.Namespace)
log.Error(err, "Failed to create Service", "name", svc.Name)
```

### RBAC markers

Add RBAC markers in the controller file alongside the `Reconcile` method. `make manifests` reads these and writes `config/rbac/role.yaml`.

```go
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsplatforms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsplatforms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsplatforms/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
```

### Authentication in AxonOpsPlatform

The `AxonAuthentication` struct defines credential priority for database components:

1. `SecretRef` — reference to an existing Kubernetes Secret (highest priority)
2. Auto-generated random credentials via `generateRandomPassword` (default fallback)

> **Note:** Inline `username` / `password` fields are not yet supported. Use `secretRef` or rely on auto-generation.

The generated password meets complexity requirements: at least one uppercase letter, one digit, and one special character.

### AxonOpsConnection for alert CRDs

Alert CRDs (`AxonOpsMetricAlert`, `AxonOpsLogAlert`, etc.) do not embed API credentials directly. Instead they reference an `AxonOpsConnection` CR by name. The shared resolver lives at `internal/controller/common/connection.go`.

---

## Auto-Generated Files — Never Edit These

Editing these files directly will be overwritten the next time `make manifests` or `make generate` runs:

| File | Generated by |
|---|---|
| `config/crd/bases/*.yaml` | `make manifests` |
| `config/rbac/role.yaml` | `make manifests` |
| `config/webhook/manifests.yaml` | `make manifests` |
| `api/**/zz_generated.deepcopy.go` | `make generate` |
| `PROJECT` | `kubebuilder` CLI |

Also, never delete `// +kubebuilder:scaffold:*` comments from any file. The kubebuilder CLI uses these as insertion points when adding new types or webhooks.

---

## Debugging

### Inspect operator logs

```bash
kubectl logs -n axonops-operator-system \
  deployment/axonops-operator-controller-manager \
  -c manager -f
```

### Describe a failing CR

```bash
kubectl describe axonopsplatform my-server -n my-namespace
kubectl get events -n my-namespace --sort-by='.lastTimestamp'
```

### Run locally with verbose output

```bash
# Increase log verbosity
go run ./cmd/main.go --zap-log-level=debug
```

### Verify CRDs are installed

```bash
kubectl get crds | grep axonops
```

### Check controller is watching the right resources

```bash
kubectl get clusterrole axonops-operator-manager-role -o yaml
```

### cert-manager issues

The AxonOpsPlatform controller creates `ClusterIssuer` and `Certificate` resources for internal database components (TimeSeries and Search). cert-manager must be installed and its CRDs must be present in the cluster when using these components.

```bash
# Install cert-manager (required for internal database components only)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Verify cert-manager is running
kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s
```

External database configurations (`spec.timeSeries.external`, `spec.search.external`) do not require cert-manager.

---

## References

- [Kubebuilder Book](https://book.kubebuilder.io) — comprehensive guide to building operators
- [controller-runtime FAQ](https://github.com/kubernetes-sigs/controller-runtime/blob/main/FAQ.md) — common patterns and gotchas
- [Good Practices](https://book.kubebuilder.io/reference/good-practices.html) — idempotency, status conditions, error handling
- [Kubebuilder Markers Reference](https://book.kubebuilder.io/reference/markers.html) — full marker syntax
- [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md) — standard field names, status conditions
- [Logging Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#message-style-guidelines) — message style and verbosity levels
