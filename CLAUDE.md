# axonops-operator - CLAUDE.md

## Project Overview

Kubernetes operator that manages the AxonOps observability stack. It aims to replace:
1. Helm charts from `axonops-containers/axonops/charts` for deploying AxonOps server components (axon-server, axon-dash, axondb-search, axondb-timeseries)
2. The Terraform provider `terraform-provider-axonops` for managing AxonOps configuration resources such as alert rules

Initial scope: implement the `AxonOpsMetricAlert` CRD to manage metric alert rules via the AxonOps REST API (replacing `resource_metric_alert_rule.go` in the Terraform provider).

@AGENTS.md

## Current Status
- **Last Updated**: 2026-03-13
- **Current Phase**: Development — Phase 1 (AxonOpsMetricAlert implementation)
- **Health**: Green — fresh scaffold, no regressions

## Active Tasks

1. [NOT STARTED] Implement `AxonOpsMetricAlert` CRD types and controller
   - Status: Not Started
   - Notes: Full spec covering all fields from the Terraform provider schema

## Recent Progress
- Initial kubebuilder scaffold committed with two CRD skeletons: `AxonOpsServer` (core group) and `AxonOpsMetricAlert` (alerts group)
- Multi-group layout (`multigroup: true`) in place; controller stubs exist

## Blockers & Issues
None currently.

---

## Architecture & Key Decisions

- **Stack**: Go 1.25.3, kubebuilder 4.13.0, controller-runtime v0.23.1, Kubernetes APIs v0.35.0
- **Layout**: Multi-group kubebuilder project (`multigroup: true`)
  - CRD group `core.axonops.com` → `api/v1alpha1/`
  - CRD group `alerts.axonops.com` → `api/alerts/v1alpha1/`
- **API versioning**: `v1alpha1` for all resources initially
- **Operator config**: AxonOps API credentials (API key, org_id, host, protocol) will be injected via a referenced `Secret` in each CR spec, or via operator-level environment variables
- **No webhooks** scaffolded yet (add later if needed for validation)

## Dependencies & Integration Points

- **AxonOps REST API** (`/api/v1/alert-rules/...`, `/api/v1/dashboardtemplate/...`): all alert rule CRD operations call this API
- **cert-manager** (optional, future): for TLS in AxonOpsServer CRD
- **Prometheus Operator** (optional, future): ServiceMonitor creation

## Useful Commands

```bash
make manifests          # Regenerate CRDs, RBAC from kubebuilder markers
make generate           # Regenerate DeepCopy methods
make fmt && make vet    # Format and vet code
make test               # Run unit tests (envtest)
make build              # Build manager binary
make run                # Run locally against current kubeconfig
```

## Known Limitations & Tech Debt
- `AxonOpsServer` spec is still the placeholder scaffold (not implemented yet)
- `AxonOpsMetricAlert` spec is still the placeholder scaffold (to be replaced in Phase 1)

---

# Implementation Plan: AxonOpsMetricAlert (Phase 1)

## Context

The Terraform provider's `resource_metric_alert_rule.go` manages metric alert rules in AxonOps clusters via a REST API. The goal is to implement the `AxonOpsMetricAlert` Kubernetes CRD and controller that provides the same CRUD lifecycle but as a native Kubernetes resource. This enables GitOps workflows and removes the Terraform dependency for alert management.

---

## Step 1 — Define `AxonOpsMetricAlert` Types

**File**: `api/alerts/v1alpha1/axonopsmetricalert_types.go`

Replace the placeholder `Foo *string` spec with the full domain model derived from the Terraform schema:

```go
// AxonOpsMetricAlertSpec defines the desired state of AxonOpsMetricAlert
type AxonOpsMetricAlertSpec struct {
    // AxonOps connection config (references a Secret with api_key)
    APISecretRef corev1.SecretReference `json:"apiSecretRef"`
    OrgID        string                 `json:"orgId"`
    // Optional: defaults to https://dash.axonops.cloud
    Host         string                 `json:"host,omitempty"`
    Protocol     string                 `json:"protocol,omitempty"` // https or http
    TLSSkipVerify bool                  `json:"tlsSkipVerify,omitempty"`

    // Alert rule identity
    ClusterName string `json:"clusterName"`
    ClusterType string `json:"clusterType"` // cassandra, kafka, dse
    Name        string `json:"name"`

    // Thresholds and evaluation
    Operator      string  `json:"operator"` // >, >=, =, !=, <=, <
    WarningValue  float64 `json:"warningValue"`
    CriticalValue float64 `json:"criticalValue"`
    Duration      string  `json:"duration"` // e.g. 15m

    // Chart/dashboard reference
    Dashboard string `json:"dashboard"`
    Chart     string `json:"chart"`
    Metric    string `json:"metric,omitempty"` // auto-derived if empty

    // Optional annotations
    // +optional
    Annotations *MetricAlertAnnotations `json:"annotations,omitempty"`

    // Optional integrations
    // +optional
    Integrations *MetricAlertIntegrations `json:"integrations,omitempty"`

    // Optional filters
    DC          []string `json:"dc,omitempty"`
    Rack        []string `json:"rack,omitempty"`
    HostID      []string `json:"hostId,omitempty"`
    Scope       []string `json:"scope,omitempty"`
    Keyspace    []string `json:"keyspace,omitempty"`
    Percentile  []string `json:"percentile,omitempty"`
    Consistency []string `json:"consistency,omitempty"`
    Topic       []string `json:"topic,omitempty"`
    GroupID     []string `json:"groupId,omitempty"`
    GroupBy     []string `json:"groupBy,omitempty"`
}

type MetricAlertAnnotations struct {
    Summary     string `json:"summary,omitempty"`
    Description string `json:"description,omitempty"`
    WidgetURL   string `json:"widgetUrl,omitempty"`
}

type MetricAlertIntegrations struct {
    Type            string   `json:"type,omitempty"`
    Routing         []string `json:"routing,omitempty"`
    OverrideInfo    bool     `json:"overrideInfo,omitempty"`
    OverrideWarning bool     `json:"overrideWarning,omitempty"`
    OverrideError   bool     `json:"overrideError,omitempty"`
}
```

**Status** stays as `Conditions []metav1.Condition` plus add:
```go
// RemoteID is the ID assigned by the AxonOps API after creation
RemoteID      string `json:"remoteId,omitempty"`
CorrelationID string `json:"correlationId,omitempty"`
```

Add kubebuilder marker for additional printer columns:
```go
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
```

After editing, run:
```bash
make generate   # regenerate zz_generated.deepcopy.go
make manifests  # regenerate CRD YAML
```

---

## Step 2 — Create AxonOps API Client

**New file**: `internal/axonops/client.go`

Implement an HTTP client mirroring `terraform-provider-axonops/client/http_client.go` but idiomatic Go (no Terraform SDK):

```go
package axonops

type Client struct {
    httpClient    *http.Client
    baseURL       string   // e.g. https://dash.axonops.cloud/orgid
    orgID         string
    apiKey        string
    tokenType     string   // "Bearer" or "AxonApi"
}

func NewClient(host, protocol, orgID, apiKey, tokenType string, tlsSkipVerify bool) *Client

// Alert rule operations
func (c *Client) ListMetricAlertRules(ctx context.Context, clusterType, clusterName string) ([]MetricAlertRule, error)
func (c *Client) CreateOrUpdateMetricAlertRule(ctx context.Context, clusterType, clusterName string, rule MetricAlertRule) (MetricAlertRule, error)
func (c *Client) DeleteMetricAlertRule(ctx context.Context, clusterType, clusterName, alertID string) error

// Dashboard lookup (for correlationId resolution)
func (c *Client) ResolveDashboardPanel(ctx context.Context, clusterType, clusterName, dashboardName, panelTitle string) (panelUUID string, metricExpr string, err error)
```

**New file**: `internal/axonops/types.go`

Contains `MetricAlertRule`, `MetricAlertAnnotations`, `MetricAlertFilter`, `MetricAlertIntegrations`, `DashboardTemplateResponse`, `Dashboard`, `DashboardPanel` structs (matching the API JSON payload from the Terraform provider).

---

## Step 3 — Implement the Controller

**File**: `internal/controller/alerts/axonopsmetricalert_controller.go`

Full reconciliation logic:

### Reconcile flow

```
1. Fetch AxonOpsMetricAlert CR
   └── If not found: return (already deleted)

2. Read API credentials from referenced Secret

3. Build axonops.Client from spec + secret

4. Handle deletion:
   └── If DeletionTimestamp set AND finalizer present:
       a. Call client.DeleteMetricAlertRule(status.RemoteID)
       b. Remove finalizer
       c. Return

5. Add finalizer if absent

6. Resolve correlationId via client.ResolveDashboardPanel(dashboard, chart)
   └── On error: set Degraded condition, return with backoff

7. Build MetricAlertRule payload from spec + correlationId

8. If status.RemoteID == "":
   a. Call client.CreateOrUpdateMetricAlertRule(...)
   b. Save returned ID to status.RemoteID
   c. Set Ready condition = True

9. Else (update path):
   a. List all rules and find by ID
   b. If drifted: call CreateOrUpdateMetricAlertRule(...)
   c. Set Ready condition = True

10. Update status subresource
```

### Key controller implementation rules (from AGENTS.md)

- Always re-fetch the CR before modifying status (use `r.Get` before `r.Status().Update`)
- Use `ctrl.Result{RequeueAfter: 30*time.Second}` for API transient errors
- Use structured logging: `log.FromContext(ctx).Info("message", "key", value)`
- Add owner references only when managing child K8s objects (not needed for pure API management)
- Use `controllerutil.AddFinalizer` / `controllerutil.RemoveFinalizer` for cleanup

### RBAC markers to add to the controller struct

```go
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
```

---

## Step 4 — Update Sample YAML

**File**: `config/samples/alerts_v1alpha1_axonopsmetricalert.yaml`

Provide a working example:

```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsMetricAlert
metadata:
  name: high-read-latency
spec:
  apiSecretRef:
    name: axonops-credentials
    namespace: default
  orgId: my-org
  clusterName: production-cluster
  clusterType: cassandra
  name: high-read-latency
  operator: ">"
  warningValue: 50
  criticalValue: 100
  duration: 15m
  dashboard: Cassandra Overview
  chart: Read Latency
  annotations:
    summary: "Cassandra read latency is high"
    description: "Read p99 latency exceeded threshold on {{ $labels.host_id }}"
  groupBy:
    - dc
    - host_id
```

Also add a sample Secret:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: axonops-credentials
  namespace: default
stringData:
  api_key: "your-api-key-here"
  token_type: "Bearer"  # or AxonApi for on-prem
```

---

## Step 5 — Write Unit Tests

**File**: `internal/controller/alerts/axonopsmetricalert_controller_test.go`

Using the existing envtest suite (`suite_test.go`), add:

1. Test create: CR created → controller calls API → status.RemoteID populated
2. Test update: spec field changed → controller detects drift → calls API again
3. Test delete: CR deleted → finalizer triggers API delete call

Use interface mocking for `axonops.Client` to avoid real HTTP calls in unit tests.

---

## Step 6 — Regenerate and Verify

```bash
make generate    # zz_generated.deepcopy.go
make manifests   # config/crd/bases/*.yaml, config/rbac/role.yaml
make fmt
make vet
make test
make build
```

---

## Critical Files

| File | Role |
|---|---|
| `api/alerts/v1alpha1/axonopsmetricalert_types.go` | CRD spec/status types |
| `api/alerts/v1alpha1/zz_generated.deepcopy.go` | Auto-generated (never edit manually) |
| `internal/axonops/client.go` | AxonOps API HTTP client |
| `internal/axonops/types.go` | API payload structs |
| `internal/controller/alerts/axonopsmetricalert_controller.go` | Controller reconciliation logic |
| `config/samples/alerts_v1alpha1_axonopsmetricalert.yaml` | Sample CR |
| `config/crd/bases/*.yaml` | Auto-generated CRD manifests |

## Reference: Terraform Provider Equivalents

| Terraform | Operator |
|---|---|
| `resource_metric_alert_rule.go` | `axonopsmetricalert_controller.go` + `axonopsmetricalert_types.go` |
| `client/http_client.go` | `internal/axonops/client.go` |
| `provider.go` auth config | `spec.apiSecretRef` + `spec.orgId` in CR |
| `terraform import` | Standard `kubectl apply` (idempotent) |

---

## Verification

1. Run `make test` — all tests pass
2. `make build` — binary compiles
3. `make manifests` — CRD YAML generated correctly at `config/crd/bases/alerts.axonops.com_axonopsmetricalerts.yaml`
4. Apply sample to a real cluster with `make install && make run` and verify:
   - CR created → `kubectl get axonopsmetricalerts` shows the resource
   - `status.remoteId` populated after reconciliation
   - Deleting the CR triggers the finalizer and removes the rule from AxonOps API
5. Inspect `kubectl describe axonopsmetricalert high-read-latency` for correct conditions

---

## Next Steps & Roadmap

1. **Phase 1** (current): `AxonOpsMetricAlert` controller — replacing `resource_metric_alert_rule.go`
2. **Phase 2**: `AxonOpsServer` controller — replacing helm charts for axon-server, axon-dash, axondb-search, axondb-timeseries
3. **Phase 3**: Additional alert/config resources from the Terraform provider (if any)
4. **Phase 4**: Webhooks for validation, conversion
5. **Phase 5**: Helm chart for the operator itself
