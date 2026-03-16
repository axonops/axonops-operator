# axonops-operator - CLAUDE.md

## Project Overview

Kubernetes operator that manages the AxonOps observability stack. It aims to replace:
1. Helm charts from `axonops-containers/axonops/charts` for deploying AxonOps server components (axon-server, axon-dash, axondb-search, axondb-timeseries)
2. The Terraform provider `terraform-provider-axonops` for managing AxonOps configuration resources such as alert rules

@AGENTS.md

## Current Status
- **Last Updated**: 2026-03-16
- **Current Phase**: Development — Phase 3 (testing & validation) active
- **Health**: Green — all CRD controllers implemented, compiled, and all unit tests passing (7/7 AxonOpsServer tests)

## Active Tasks

1. [COMPLETED] Fix test environment to handle external database support without cert-manager
   - Status: Completed
   - Completion: All unit tests for external database configurations now pass (Phase 1 & Phase 2)
   - Solution: Made cert-manager optional via refactored needsInternalResources() check; tests use mockRESTMapper
2. [IN PROGRESS] End-to-end testing of AxonOpsServer controller
   - Status: In Progress
   - Notes: Against a real Kind cluster; verify StatefulSets, Ingress, and Gateway resources for internal and external modes
3. [NOT STARTED] Unit tests for alert controllers (LogAlert, AlertRoute, Healthchecks)
   - Status: Not Started
   - Notes: AxonOpsMetricAlert has tests; remaining alert controllers need test coverage
4. [NOT STARTED] Webhooks for CRD validation
   - Status: Not Started
   - Notes: Useful for field validation (e.g., operator enum, clusterType enum)

## Recent Progress
- **AxonOpsServer controller fully implemented** (2,636 lines):
  - TimeSeries StatefulSet with auth secrets (`AXONOPS_DB_USER`, `AXONOPS_DB_PASSWORD`)
  - Search StatefulSet with auth secrets (`AXONOPS_SEARCH_USER`, `AXONOPS_SEARCH_PASSWORD`)
  - Server StatefulSet with config secret and TLS certificate management
  - Dashboard Deployment with ConfigMap
  - Ingress support for dashboard, server agent port, and server API port
  - Gateway API support (Gateway + HTTPRoute) for dashboard, server agent, and server API
  - Auto-generated passwords meeting complexity requirements (`generateRandomPassword`)
  - ServiceAccounts and headless + ClusterIP Services per component
  - TLS keystore password secrets and self-signed certificate lifecycle
- **All six alert CRDs fully implemented**:
  - `AxonOpsMetricAlert` — metric threshold alerts (controller + tests + sample)
  - `AxonOpsLogAlert` — log pattern alerts (controller + sample, reuses metric alert API endpoint with events{...} syntax)
  - `AxonOpsAlertRoute` — alert routing/notification channels (controller + sample)
  - `AxonOpsHealthcheckHTTP` — HTTP healthcheck alerts (controller + sample)
  - `AxonOpsHealthcheckShell` — Shell script healthchecks (controller + sample)
  - `AxonOpsHealthcheckTCP` — TCP port healthchecks (controller + sample)
- **`AxonOpsConnection` CRD** added to `core.axonops.com` group for reusable API auth config
- AxonOps API client (`internal/axonops/client.go` + `types.go`) implemented
- Sample YAMLs for all CRDs under `config/samples/`

## Blockers & Issues
None currently.

---

## Architecture & Key Decisions

- **Stack**: Go 1.25.3, kubebuilder 4.13.0, controller-runtime v0.23.1, Kubernetes APIs v0.35.0
- **Layout**: Multi-group kubebuilder project (`multigroup: true`)
  - CRD group `core.axonops.com` → `api/v1alpha1/` — `AxonOpsServer`, `AxonOpsConnection`
  - CRD group `alerts.axonops.com` → `api/alerts/v1alpha1/` — all six alert CRDs
- **API versioning**: `v1alpha1` for all resources initially
- **Auth strategy**:
  - `AxonOpsConnection` CR holds org-level API credentials (orgId, apiKeyRef secret, host, protocol, tokenType, tlsSkipVerify, useSAML)
  - Alert CRs reference an `AxonOpsConnection` by name rather than embedding credentials
- **AxonOpsServer auth**: database credentials use `AxonAuthentication` — priority is `SecretRef` > explicit username/password > auto-generated random credentials
- **Password policy**: `generateRandomPassword` enforces at least one uppercase, one digit, one special character
- **Ingress + Gateway**: both `Ingress` and `GatewayConfig` structs are supported on Dashboard, Server Agent, and Server API endpoints; they are independent and can be enabled together or separately
- **No webhooks** scaffolded yet (add later if needed for validation)

---

## Development Workflow

### Feature Request Process

When requesting a new feature, follow this structured workflow:

**Phase 1: GitHub Issue**
- Create a GitHub issue with clear title and description
- Include problem statement and proposed solution
- Add appropriate labels (`feature`, `bug`, `enhancement`, etc.)
- Note the issue number (e.g., #42)

**Phase 2: Planning Session**
- Use `EnterPlanMode` to design the implementation
- Explore codebase impact and affected files
- Identify dependencies and potential risks
- Get approval on the approach before coding

**Phase 3: Feature Branch**
- Create branch with format: `<type>/<issue-number>-<slug>`
- Examples: `feature/42-webhook-support`, `fix/15-namespace-leak`, `docs/20-api-docs`
- Command: `git checkout -b feature/42-webhook-support`

**Phase 3.5: BDD Test Specification (required before implementation)**
- Before writing any implementation code, write BDD test files under `test/bdd/`
- Each GitHub issue with BDD scenarios must have a corresponding `.feature` file (Gherkin format)
- File naming: `test/bdd/<feature-slug>.feature` (e.g., `test/bdd/external-timeseries.feature`, `test/bdd/external-searchdb.feature`)
- The `.feature` file must contain all Gherkin scenarios from the GitHub issue
- Commit the BDD test files to the feature branch before starting implementation
- This ensures acceptance criteria are codified and reviewable before code is written

**Phase 4: Implementation**
- Follow the approved plan from EnterPlanMode
- Keep commits focused with clear messages
- Run quality checks: `make fmt && make vet && make lint && make test`
- All tests must pass before submitting PR

**Phase 5: Commit & PR**
- Use conventional commit format with issue reference:
  ```
  feat: add webhook support for alert notifications

  - Implement HTTPServer for webhook endpoint
  - Add WebhookConfig CRD for routing
  - Support authentication via API keys

  Closes #42
  ```
- Create PR with title describing the change
- Body should include `Closes #<issue-number>` to auto-link
- Ensure CI checks pass

**Phase 6: Merge & Deploy**
- Get approval from maintainers
- Merge to main branch
- GitHub issue closes automatically

### Branch Naming Convention

| Type | Format | Example |
|---|---|---|
| Feature | `feature/<number>-<slug>` | `feature/42-webhook-support` |
| Bug Fix | `fix/<number>-<slug>` | `fix/15-namespace-leak` |
| Documentation | `docs/<number>-<slug>` | `docs/20-api-docs` |
| Chore | `chore/<number>-<slug>` | `chore/8-upgrade-deps` |

### Workflow Example

```
1. User: "Add webhook support for alerts"
2. Create GitHub issue #42
3. Launch EnterPlanMode for design approval
4. git checkout -b feature/42-webhook-support
5. Implement per approved plan
6. make fmt && make vet && make lint && make test
7. git commit -m "feat: add webhook support..." Closes #42
8. Create PR, wait for CI + review
9. Merge to main
10. GitHub issue #42 auto-closes
```

---

## Dependencies & Integration Points

- **AxonOps REST API** (`/api/v1/alert-rules/...`, `/api/v1/dashboardtemplate/...`, `/api/v1/integrations/...`, `/api/v1/healthchecks/...`): all alert CRD controllers call this API
- **Kubernetes Gateway API** (`gateway.networking.k8s.io`): used by AxonOpsServer for Gateway/HTTPRoute resources
- **cert-manager** (optional): for TLS certificate management in AxonOpsServer; only required when using internal database components (TimeSeries/Search StatefulSets); not needed for external database configurations
- **Prometheus Operator** (optional, future): ServiceMonitor creation

## API Groups & CRDs

### `core.axonops.com` (`api/v1alpha1/`)
| CRD | File | Status |
|---|---|---|
| `AxonOpsServer` | `axonopsserver_types.go` | ✅ Implemented |
| `AxonOpsConnection` | `axonopsconnection_types.go` | ✅ Implemented |

### `alerts.axonops.com` (`api/alerts/v1alpha1/`)
| CRD | File | Status |
|---|---|---|
| `AxonOpsMetricAlert` | `axonopsmetricalert_types.go` | ✅ Implemented + tested |
| `AxonOpsLogAlert` | `axonopslogalert_types.go` | ✅ Implemented |
| `AxonOpsAlertRoute` | `axonopsalertroute_types.go` | ✅ Implemented |
| `AxonOpsHealthcheckHTTP` | `axonopshealthcheckhttp_types.go` | ✅ Implemented |
| `AxonOpsHealthcheckShell` | `axonopshealthcheckshell_types.go` | ✅ Implemented |
| `AxonOpsHealthcheckTCP` | `axonopshealthchecktcp_types.go` | ✅ Implemented |

## AxonOpsServer Controller — Key Resources Managed

Each `AxonOpsServer` CR reconciles the following Kubernetes objects (all owned via `SetControllerReference`):

| Component | Resources Created |
|---|---|
| **TimeSeries** (`axondb-timeseries`) | ServiceAccount, headless Service, ClusterIP Service, auth Secret, keystore Secret, TLS cert Secret, StatefulSet |
| **Search** (`axondb-search`) | ServiceAccount, headless Service, ClusterIP Service, auth Secret, keystore Secret, TLS cert Secret, StatefulSet |
| **Server** (`axon-server`) | ServiceAccount, agent Service, API Service, config Secret, StatefulSet, Ingress (agent), Ingress (API), Gateway+HTTPRoute (agent), Gateway+HTTPRoute (API) |
| **Dashboard** (`axon-dash`) | ServiceAccount, ClusterIP Service, ConfigMap, Deployment, Ingress, Gateway+HTTPRoute |

## Useful Commands

```bash
make manifests          # Regenerate CRDs, RBAC from kubebuilder markers
make generate           # Regenerate DeepCopy methods
make fmt && make vet    # Format and vet code
make test               # Run unit tests (envtest)
make build              # Build manager binary
make run                # Run locally against current kubeconfig
make install            # Install CRDs into cluster
kubectl apply -k config/samples/   # Apply sample CRs
```

## Known Limitations & Tech Debt
- Alert controllers other than `AxonOpsMetricAlert` do not have unit tests yet (wired and compiling, awaiting test coverage)
- TLS certificates use self-signed certificates via cert-manager; cert-manager is optional (only required for internal database/workload components)
- No webhooks for field validation (e.g., `clusterType` enum enforcement, `operator` enum enforcement)
- Gateway API CRDs must be pre-installed in the cluster (not bundled with the operator)
- `AxonOpsServer` TLS certificate rotation and renewal is basic (no automatic renewal lifecycle)

---

## Next Steps & Roadmap

1. **Phase 3** (in progress): Testing & hardening
   - ✅ Unit tests for AxonOpsServer external database configurations (all 7 tests passing)
   - E2E tests for `AxonOpsServer` against a Kind cluster (real cluster scenarios)
   - Unit tests for remaining alert controllers (LogAlert, AlertRoute, Healthchecks)
   - Validate Ingress and Gateway API resources end-to-end
2. **Phase 4**: Webhooks for validation and defaulting
3. **Phase 5**: Helm chart for the operator itself (`kubebuilder edit --plugins=helm/v2-alpha`)
4. **Phase 6**: Enhanced cert-manager integration for AxonOpsServer TLS (e.g., cert renewal, custom CA issuers)
5. **Phase 7**: Additional Terraform provider resources if needed

## Reference: Terraform Provider Equivalents

| Terraform Resource | Operator CRD |
|---|---|
| `resource_metric_alert_rule.go` | `AxonOpsMetricAlert` |
| `resource_log_alert_rule.go` | `AxonOpsLogAlert` |
| `resource_alert_route.go` | `AxonOpsAlertRoute` |
| `resource_healthcheck_http.go` | `AxonOpsHealthcheckHTTP` |
| `resource_healthcheck_shell.go` | `AxonOpsHealthcheckShell` |
| `resource_healthcheck_tcp.go` | `AxonOpsHealthcheckTCP` |
| `provider.go` auth config | `AxonOpsConnection` CR |
| Helm chart (axon-server, axon-dash, axondb-*) | `AxonOpsServer` CR |
