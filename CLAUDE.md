# axonops-operator - CLAUDE.md

## Project Overview

Kubernetes operator that manages the AxonOps observability stack. It aims to replace:
1. Helm charts from `axonops-containers/axonops/charts` for deploying AxonOps server components (axon-server, axon-dash, axondb-search, axondb-timeseries)
2. The Terraform provider `terraform-provider-axonops` for managing AxonOps configuration resources such as alert rules

@AGENTS.md

## Current Status
- **Last Updated**: 2026-03-16
- **Current Phase**: Development — Phase 3 (testing & validation) with code review findings
- **Health**: Green — all CRD controllers implemented and compiled; identified 7 bugs in code review (#20-#26)

## Workflow — Agent Gates

These agents are mandatory gates, not optional tools. Do not skip them.

### Before creating any GitHub issue:
Run the **issue-writer** agent. Every issue must have: summary, detailed requirements, numbered acceptance criteria, specific testing requirements (named tests, not "add tests"), documentation requirements, dependencies, and labels. If any section is missing or vague, rewrite it before creating.

### After completing any feature:
1. **code-reviewer** — on all changed files
2. **security-reviewer** — on any code touching TLS, HTTP, credentials, or external input
3. **go-quality** — as a final gate before commit

### After creating or modifying CI/CD configuration:
4. **devops** — on any workflow, GoReleaser, Dependabot, or Makefile changes

### When writing tests:
5. **test-writer** — use for creating unit, integration, and BDD tests

### When writing or reviewing documentation:
6. **docs-writer** — on any README, godoc, examples, CONTRIBUTING, CHANGELOG, SECURITY.md, or config reference changes. All documentation follows RFC 2119 for requirement-level terminology. Every code example must compile. Every config field must be documented with type, default, valid values, and boundary behaviour.


## Active Tasks

### Code Review Findings (Priority Bugs)

1. [NOT STARTED] Issue #20 (HIGH): External username/password path sets empty secret name
   - Location: `internal/controller/axonopsserver_controller.go:376,433`
   - Status: Not Started
   - Notes: External auth path not implemented; either document as unsupported or implement managed Secret creation

2. [NOT STARTED] Issue #21 (HIGH): Nil pointer dereference when TimeSeries/Search enabled without Server
   - Location: `internal/controller/axonopsserver_controller.go:1288`
   - Status: Not Started
   - Notes: Missing nil-guard on `server.Spec.Server.OrgName`; add default orgName or return error

3. [NOT STARTED] Issue #22 (MEDIUM): Template errors in buildServerConfig produce silent broken config
   - Location: `internal/controller/axonopsserver_controller.go:2211–2219`
   - Status: Not Started
   - Notes: Function returns string instead of error; needs signature change and error propagation

4. [NOT STARTED] Issue #23 (MEDIUM): Disabled components leave orphaned Kubernetes resources
   - Location: `internal/controller/axonopsserver_controller.go:347,404,461,470`
   - Status: Not Started
   - Notes: Need cleanup helpers for StatefulSets, Services, Secrets, TLS Certs when component disabled

5. [NOT STARTED] Issue #24 (MEDIUM): Ingress and Gateway resources never cleaned up when disabled
   - Location: `internal/controller/axonopsserver_controller.go` — `reconcileServer`, `reconcileDashboard`
   - Status: Not Started
   - Notes: Implement `cleanupIngress` and `cleanupGateway` helpers

6. [NOT STARTED] Issue #25 (MEDIUM): PVCs not deleted when switching to external database
   - Location: `internal/controller/axonopsserver_controller.go:899–970`
   - Status: Not Started
   - Notes: After StatefulSet deletion, enumerate and delete PVCs with pattern `data-<name>-<component>-<ordinal>`

7. [NOT STARTED] Issue #26 (LOW): Enabled bool with default=true defaults to false at runtime
   - Location: `api/v1alpha1/axonopsserver_types.go:293`
   - Status: Not Started
   - Notes: Change `Enabled` to `*bool` with proper defaulting webhook or validation

### Planned Features

8. [NOT STARTED] Issue #27 (FEATURE): Add Helm chart for operator installation
   - Status: Not Started
   - Target: Phase 5 of roadmap
   - BDD File: `helm-chart-installation.feature` (15 scenarios)
   - Notes: Use `kubebuilder edit --plugins=helm/v2-alpha` to scaffold chart with configurable values, RBAC, and webhook support

### Testing & Validation

9. [COMPLETED] BDD Feature Files (12 total)
   - Status: Completed
   - Files: internal-deployment.feature, authentication-lifecycle.feature, component-lifecycle.feature, ingress-gateway.feature, axonops-connection.feature, alert-metric-alert.feature, alert-log-alert.feature, alert-route.feature, healthcheck-http.feature, healthcheck-tcp.feature, healthcheck-shell.feature, helm-chart-installation.feature
   - Committed with: `docs: add BDD feature files for all implemented features`

9. [IN PROGRESS] End-to-end testing of AxonOpsServer controller
   - Status: In Progress
   - Notes: Against a real Kind cluster; verify StatefulSets, Ingress, and Gateway resources for internal and external modes

10. [NOT STARTED] Unit tests for alert controllers (LogAlert, AlertRoute, Healthchecks)
    - Status: Not Started
    - Notes: AxonOpsMetricAlert has tests; remaining alert controllers need test coverage

11. [NOT STARTED] Webhooks for CRD validation
    - Status: Not Started
    - Notes: Useful for field validation (e.g., operator enum, clusterType enum)

## Recent Progress
- **Comprehensive Code Review Completed (2026-03-16)**:
  - Identified 7 actionable bugs across AxonOpsServer controller (#20-#26)
  - Created GitHub issues with detailed descriptions and fix scope
  - Issues span: credential handling, nil pointer dereference, template error handling, resource cleanup, component disable transitions, PVC cleanup, and bool default handling
- **BDD Feature Files Written (11 total)**:
  - `internal-deployment.feature` — Full 4-component deployment happy path
  - `authentication-lifecycle.feature` — SecretRef, inline credentials, auto-generated passwords
  - `component-lifecycle.feature` — Enable/disable with resource cleanup
  - `ingress-gateway.feature` — Ingress and Gateway API configurations
  - `axonops-connection.feature` — AxonOpsConnection lifecycle
  - `alert-metric-alert.feature` — AxonOpsMetricAlert CRUD with API sync
  - `alert-log-alert.feature` — AxonOpsLogAlert CRUD with event patterns
  - `alert-route.feature` — AxonOpsAlertRoute integration routing
  - `healthcheck-http.feature` — HTTP healthcheck lifecycle
  - `healthcheck-tcp.feature` — TCP healthcheck lifecycle
  - `healthcheck-shell.feature` — Shell script healthcheck lifecycle
  - Committed: `docs: add BDD feature files for all implemented features`
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
  - `AxonOpsLogAlert` — log pattern alerts (controller + sample)
  - `AxonOpsAlertRoute` — alert routing/notification channels (controller + sample)
  - `AxonOpsHealthcheckHTTP` — HTTP healthcheck alerts (controller + sample)
  - `AxonOpsHealthcheckShell` — Shell script healthchecks (controller + sample)
  - `AxonOpsHealthcheckTCP` — TCP port healthchecks (controller + sample)
- **`AxonOpsConnection` CRD** added to `core.axonops.com` group for reusable API auth config
- AxonOps API client (`internal/axonops/client.go` + `types.go`) implemented
- Sample YAMLs for all CRDs under `config/samples/`

## Blockers & Issues

### Code Review Identified Bugs (GitHub Issues #20-#26)
7 bugs identified during comprehensive code review:
- **#20 (HIGH)**: External credential path stores empty secret name
- **#21 (HIGH)**: Nil pointer dereference when components enabled without Server
- **#22 (MEDIUM)**: Template errors produce silent broken config
- **#23 (MEDIUM)**: Disabled components leave orphaned resources
- **#24 (MEDIUM)**: Ingress/Gateway not cleaned up when disabled
- **#25 (MEDIUM)**: PVCs not deleted on mode switch to external DB
- **#26 (LOW)**: Bool default=true defaults to false at runtime

See "Active Tasks" section above for details and fix scope for each issue.

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

### Critical Bugs (See Issues #20-#26)
- **#20**: External database credential path doesn't properly create managed Secrets
- **#21**: Nil pointer dereference when database components enabled without Server component
- **#22**: Template rendering errors swallowed silently; broken config stored as valid Secret
- **#23**: Disabling components doesn't clean up existing StatefulSets, Services, Secrets
- **#24**: Disabling Ingress/Gateway doesn't delete existing resources
- **#25**: PVCs not cleaned up when switching to external database mode
- **#26**: `Enabled bool` fields default to `false` at runtime despite `default=true` marker

### Other Limitations
- Alert controllers other than `AxonOpsMetricAlert` do not have unit tests yet (wired and compiling, awaiting test coverage)
- TLS certificates use self-signed certificates via cert-manager; cert-manager is optional (only required for internal database/workload components)
- No webhooks for field validation (e.g., `clusterType` enum enforcement, `operator` enum enforcement)
- Gateway API CRDs must be pre-installed in the cluster (not bundled with the operator)
- `AxonOpsServer` TLS certificate rotation and renewal is basic (no automatic renewal lifecycle)

---

## Next Steps & Roadmap

1. **Phase 3 (active)**: Fix identified bugs and improve test coverage
   - Fix 7 identified bugs (Issues #20-#26) following planned fix scope
   - E2E tests for `AxonOpsServer` against a Kind cluster (real cluster scenarios)
   - Unit tests for remaining alert controllers (LogAlert, AlertRoute, Healthchecks)
   - Validate Ingress and Gateway API resources end-to-end
   - Write step definitions for BDD feature files (Godog or similar integration)

2. **Phase 4**: Webhooks for validation and defaulting
   - Defaulting webhook for `Enabled` bool fields (Issue #26)
   - Validation webhooks for enum fields (clusterType, operator, integrationType)

3. **Phase 5**: Helm chart for the operator itself (Issue #27)
   - Generate scaffolding with `kubebuilder edit --plugins=helm/v2-alpha`
   - Configure values.yaml for image, replicas, resource limits, log level
   - Document installation, configuration, and upgrade procedures

4. **Phase 6**: Enhanced cert-manager integration for AxonOpsServer TLS
   - Optional cert-manager integration (already optional, needs testing)
   - Certificate renewal and rotation lifecycle
   - Custom CA issuer support

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
