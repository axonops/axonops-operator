# axonops-operator - CLAUDE.md

## Project Overview

Kubernetes operator that manages the AxonOps observability stack. It aims to replace:
1. Helm charts from `axonops-containers/axonops/charts` for deploying AxonOps server components (axon-server, axon-dash, axondb-search, axondb-timeseries) - IMPORTANT: after each CRD change, ensure the helm chart is updated at `charts/axonops-operator/templates/crd/` and `charts/axonops-operator/templates/rbac/`.
2. The Terraform provider `terraform-provider-axonops` for managing AxonOps configuration resources such as alert rules
3. The Ansible collection `axonops-ansible-collection` for managing backups, repairs, silences, and other operational resources

@AGENTS.md

## Current Status
- **Last Updated**: 2026-03-30
- **Current Phase**: Development — Feature implementation + code quality improvements
- **Health**: Green — 4 API groups, 20+ CRDs implemented with controllers and tests

## Workflow — Agent Gates

These agents are mandatory gates, not optional tools. Do not skip them.

### Before creating any GitHub issue:
Run the **issue-writer** agent. Every issue must have: summary, detailed requirements, numbered acceptance criteria, specific testing requirements (named tests, not "add tests"), documentation requirements, dependencies, and labels. If any section is missing or vague, rewrite it before creating.

### Before pushing to GitHub

Run `make lint` and fix **all** linting issues. Do not push code with lint errors.

### After completing any feature:
1. **code-reviewer** — on all changed files
2. **security-reviewer** — on any code touching TLS, HTTP, credentials, or external input
3. **go-quality** — as a final gate before commit

### After creating or modifying CI/CD configuration:
4. **devops** — on any workflow, GoReleaser, Dependabot, or Makefile changes

### When writing tests:
5. **test-writer** — use for creating unit, integration, and BDD tests

### When writing or reviewing documentation:
6. **docs-writer** — on any README, godoc, examples, CONTRIBUTING, CHANGELOG, SECURITY.md, or config reference changes.

### When working on complex Kubernetes tasks
7. Ask **kube** for help with kubebuilder, operators or decisions about how to implement and resolve Kubernetes issues.

### Helm chart sync (CRITICAL)
After ANY CRD or RBAC change:
1. Copy CRDs: `cp config/crd/bases/*.yaml charts/axonops-operator/templates/crd/`
2. Copy RBAC roles: copy from `config/rbac/*_role.yaml` to `charts/axonops-operator/templates/rbac/` (convert `_` to `-` in filenames)
3. Update manager role: sync rules from `config/rbac/role.yaml` into `charts/axonops-operator/templates/rbac/manager-role.yaml`

## Active Tasks

### Code Quality Issues (from comprehensive review #72-#77)

1. [COMPLETED] Issue #72 (P0 SECURITY): URL path injection in API client — all path segments now escaped with `url.PathEscape()`
2. [COMPLETED] Issue #73 (P1): Bare type assertions replaced with `errors.As` (32 occurrences); bubble sort replaced with `slices.Sort`
3. [NOT STARTED] Issue #74 (P2): Extract shared controller helpers to common package (`condTypeReady`, `setFailedCondition`, test helpers)
4. [NOT STARTED] Issue #75 (P3): Quick code quality fixes (routeTypeMap encoding, scaffold TODOs, Go acronym casing)
5. [NOT STARTED] Issue #76 (P1): Add unit tests for internal/axonops API client (currently 7.7% coverage)
6. [NOT STARTED] Issue #77 (P1): Split axonopsplatform_controller.go into component files (3618 lines)

### Rename (breaking change)
7. [COMPLETED] Issue #106 (P1): `AxonOpsServer` CRD renamed to `AxonOpsPlatform` — all files, CRDs, RBAC, Helm, docs updated

### Original Bugs (#20-#26)
- **#20 (HIGH)**: External credential path stores empty secret name — NOT STARTED
- **#21 (HIGH)**: Nil pointer dereference when components enabled without Server — NOT STARTED
- **#22 (MEDIUM)**: Template errors produce silent broken config — NOT STARTED
- **#23 (MEDIUM)**: Disabled components leave orphaned resources — NOT STARTED
- **#24 (MEDIUM)**: Ingress/Gateway not cleaned up when disabled — NOT STARTED
- **#25 (MEDIUM)**: PVCs not deleted on mode switch to external DB — NOT STARTED
- **#26 (LOW)**: Bool default=true defaults to false at runtime — NOT STARTED

## Recent Progress (2026-03-18 to 2026-03-30)

### New CRD Groups & Controllers Implemented
- **`backups.axonops.com`** — `AxonOpsBackup` (S3/SFTP/Azure, inline + SecretRef credentials, PR #65)
- **`kafka.axonops.com`** — `AxonOpsKafkaTopic` (PR #69), `AxonOpsKafkaACL` (PR #70), `AxonOpsKafkaConnector` (PR #71)
- **`alerts.axonops.com` additions** — `AxonOpsScheduledRepair` (PR #67), `AxonOpsSilenceWindow` (PR #90)

### Breaking Changes
- **Renamed `AxonOpsServer` → `AxonOpsPlatform`** (issue #106) — all Go types, CRDs, RBAC, Helm chart, samples, BDD tests, docs updated; migration guide in #106

### Infrastructure Improvements
- Extracted `ResolveAPIClient` to shared `internal/controller/common/` package (PR #65)
- Added `url.PathEscape()` to all API client URL constructions (PR #79, closes #72)
- Replaced 32 bare `err.(*axonops.APIError)` with `errors.As` (PR #78, closes #73)
- Replaced custom bubble sort with `slices.Sort` (PR #78)
- Sanitized `ResolveDashboardPanel` error messages (no more orgID/response body leak)
- Added first API client unit tests (`internal/axonops/client_test.go`)
- Config hash stability fix — compute hashes from source data, not re-read Secrets (PR #62)
- Config rolling updates via checksum annotations on pod templates (PR #58)
- Global `spec.imageRegistry` for on-premises registry override (PR #61)
- Pinned init container image with `spec.initImage` (PR #59)
- Synced Helm chart CRDs and RBAC for all new resources (PR #89)
- Added configurable HTTP client timeout via `AxonOpsConnection.spec.timeout`
- Added OpenTelemetry tracing and Prometheus metrics to all controllers

### Test Coverage
- Alert controller tests: 6 controllers × 6 scenarios each (PR #57)
- Backup controller tests: 8 scenarios including S3/SFTP/Azure
- Kafka controller tests: Topic (5), ACL (4), Connector (4)
- Scheduled repair tests: 6 scenarios
- Silence window tests: 4 scenarios
- API client tests: 5 tests (path escaping, validation, error classification)

---

## Architecture & Key Decisions

- **Stack**: Go 1.25.3, kubebuilder 4.13.0, controller-runtime v0.23.1, Kubernetes APIs v0.35.0
- **Layout**: Multi-group kubebuilder project (`multigroup: true`)
- **API versioning**: `v1alpha1` for all resources
- **Shared infrastructure**: `internal/controller/common/connection.go` — `ResolveAPIClient`, `BuildHostURL`, shared condition constants
- **Credential pattern**: `SecretRef` > inline fields > default auth (IAM/MSI) — consistent across all CRDs
- **API client security**: All URL path segments escaped with `url.PathEscape()`; `errors.As` for error discrimination

---

## API Groups & CRDs

### `core.axonops.com` (`api/v1alpha1/`)
| CRD | Status | Notes |
|---|---|---|
| `AxonOpsPlatform` | ✅ Implemented + tested | 4-component deployment (TimeSeries, Search, Server, Dashboard) |
| `AxonOpsConnection` | ✅ Implemented | Shared API auth config with configurable timeout |

### `alerts.axonops.com` (`api/alerts/v1alpha1/`)
| CRD | Status | Notes |
|---|---|---|
| `AxonOpsMetricAlert` | ✅ Implemented + tested | Metric threshold alerts |
| `AxonOpsLogAlert` | ✅ Implemented + tested | Log pattern alerts |
| `AxonOpsAlertRoute` | ✅ Implemented + tested | Alert routing to integrations |
| `AxonOpsHealthcheckHTTP` | ✅ Implemented + tested | HTTP healthchecks |
| `AxonOpsHealthcheckShell` | ✅ Implemented + tested | Shell script healthchecks |
| `AxonOpsHealthcheckTCP` | ✅ Implemented + tested | TCP port healthchecks |
| `AxonOpsAlertEndpoint` | ✅ Implemented + tested | Integration endpoint definitions |
| `AxonOpsAdaptiveRepair` | ✅ Implemented + tested | Adaptive repair settings |
| `AxonOpsScheduledRepair` | ✅ Implemented + tested | Cron-based scheduled repairs |
| `AxonOpsDashboardTemplate` | ✅ Implemented + tested | Declarative dashboard management |
| `AxonOpsCommitlogArchive` | ✅ Implemented + tested | Commitlog archive settings |
| `AxonOpsSilenceWindow` | ✅ Implemented + tested | Alert silence windows |

### `backups.axonops.com` (`api/backups/v1alpha1/`)
| CRD | Status | Notes |
|---|---|---|
| `AxonOpsBackup` | ✅ Implemented + tested | Cassandra scheduled snapshots (S3/SFTP/Azure) |

### `kafka.axonops.com` (`api/kafka/v1alpha1/`)
| CRD | Status | Notes |
|---|---|---|
| `AxonOpsKafkaTopic` | ✅ Implemented + tested | Topic lifecycle + config management |
| `AxonOpsKafkaACL` | ✅ Implemented + tested | Kafka ACL entry management |
| `AxonOpsKafkaConnector` | ✅ Implemented + tested | Kafka Connect connector management |

## Dependencies & Integration Points

- **AxonOps REST API**: All CRD controllers call this API (alert rules, healthchecks, integrations, backups, repairs, silences, Kafka topics/ACLs/connectors)
- **Kubernetes Gateway API** (`gateway.networking.k8s.io`): used by AxonOpsPlatform for Gateway/HTTPRoute resources
- **cert-manager** (optional): for TLS certificate management in AxonOpsPlatform internal components
- **Prometheus Operator** (optional): ServiceMonitor creation via OTel metrics

## Useful Commands

```bash
make manifests          # Regenerate CRDs, RBAC from kubebuilder markers
make generate           # Regenerate DeepCopy methods
make fmt && make vet    # Format and vet code
make lint               # Run golangci-lint
make test               # Run unit tests (envtest)
make build              # Build manager binary
make run                # Run locally against current kubeconfig
make install            # Install CRDs into cluster
kubectl apply -k config/samples/   # Apply sample CRs
```

## Known Limitations & Tech Debt

### Critical (Issues #20-#26)
- External database credential path, nil pointer dereference, template error handling, resource cleanup on disable, PVC cleanup, bool defaulting

### Code Quality (Issues #72-#77)
- Server controller is 3618 lines (needs splitting into component files)
- `condTypeReady` duplicated in 3 packages (needs extraction to common)
- Inconsistent status condition patterns (Failed type vs Ready=False)
- Test helper duplication across 3 packages
- `internal/axonops/` at only 7.7% test coverage

### Other
- No webhooks for field validation
- Gateway API CRDs must be pre-installed in the cluster
- `BuildHostURL` hardcodes `https://`, ignoring Connection protocol field
- New API client created on every reconciliation (no connection reuse)

---

## Next Steps & Roadmap

1. **Phase 3 (active)**: Code quality + test coverage
   - Fix remaining bugs #20-#26
   - Split server controller (#77)
   - Extract shared helpers to common (#74)
   - Increase API client test coverage (#76)
   - E2E tests against Kind cluster

2. **Phase 4**: Webhooks for validation and defaulting

3. **Phase 5**: Helm chart improvements (Issue #27)
   - Global `imageRegistry` support (implemented, Issue #60)
   - Configurable values for all operator settings

4. **Phase 6**: Enhanced cert-manager integration

## Reference: Source Equivalents

| Source | Operator CRD |
|---|---|
| TF `resource_metric_alert_rule.go` | `AxonOpsMetricAlert` |
| TF `resource_log_alert_rule.go` | `AxonOpsLogAlert` |
| TF `resource_alert_route.go` | `AxonOpsAlertRoute` |
| TF `resource_healthcheck_http.go` | `AxonOpsHealthcheckHTTP` |
| TF `resource_healthcheck_shell.go` | `AxonOpsHealthcheckShell` |
| TF `resource_healthcheck_tcp.go` | `AxonOpsHealthcheckTCP` |
| TF `resource_cassandra_scheduled_repair.go` | `AxonOpsScheduledRepair` |
| TF `resource_cassandra_adaptive_repair.go` | `AxonOpsAdaptiveRepair` |
| TF `resource_kafka_topic.go` | `AxonOpsKafkaTopic` |
| TF `resource_kafka_acl.go` | `AxonOpsKafkaACL` |
| TF `resource_kafka_connect_connector.go` | `AxonOpsKafkaConnector` |
| Ansible `backup.py` | `AxonOpsBackup` |
| Ansible `silence.py` | `AxonOpsSilenceWindow` |
| TF `provider.go` auth config | `AxonOpsConnection` CR |
| Helm chart (axon-server, axon-dash, axondb-*) | `AxonOpsPlatform` CR |
