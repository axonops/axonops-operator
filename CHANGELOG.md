# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.1.0] — 2026-04-09

Initial public release of the AxonOps Kubernetes Operator.

### Added

#### Infrastructure (`core.axonops.com`)
- `AxonOpsPlatform` CRD — deploys and manages the full AxonOps server stack (axon-server, axon-dash, axondb-timeseries, axondb-search) from a single declarative resource
- `AxonOpsConnection` CRD — stores reusable AxonOps API credentials referenced by alert and backup CRDs
- Internal database components: operator-managed StatefulSets with automatic credential generation and TLS certificate provisioning via cert-manager
- External database support: connect axon-server to user-managed Cassandra and Elasticsearch/OpenSearch clusters
- TLS for external connections: `spec.*.external.tls.certSecretRef` — required when `tls.enabled=true` and `tls.insecureSkipVerify=false`; omitting it sets `Ready=False` with reason `MissingExternalTLSCert`
- `spec.imageRegistry` — global registry override for all default component images (on-premises support)
- `spec.initImage` — configurable init container image (default: `docker.io/library/busybox:1.37.0`)
- `spec.pause` — halts reconciliation without deleting owned resources
- Startup ordering: Server waits for databases to be ready; Dashboard waits for Server
- Config rolling updates via checksum annotations on pod templates
- Ingress and Gateway API support for Server (agent and API endpoints) and Dashboard
- `spec.server.license.secretRef` / `spec.server.license.key` for license configuration
- `spec.server.config` for additional axon-server configuration (merged into generated config)

#### Alerting and operations (`alerts.axonops.com`)
- `AxonOpsMetricAlert` — metric threshold alert rules
- `AxonOpsLogAlert` — log pattern alert rules
- `AxonOpsAlertRoute` — alert routing to notification channels
- `AxonOpsAlertEndpoint` — integration endpoint definitions (PagerDuty, Slack, email, OpsGenie, etc.)
- `AxonOpsHealthcheckHTTP` — HTTP endpoint healthchecks
- `AxonOpsHealthcheckTCP` — TCP port healthchecks
- `AxonOpsHealthcheckShell` — shell script healthchecks
- `AxonOpsDashboardTemplate` — declarative dashboard management
- `AxonOpsAdaptiveRepair` — adaptive Cassandra repair configuration
- `AxonOpsScheduledRepair` — cron-based Cassandra scheduled repairs
- `AxonOpsCommitlogArchive` — commitlog archive settings
- `AxonOpsSilenceWindow` — alert silence windows
- `AxonOpsLogCollector` — log collector configuration

#### Backups (`backups.axonops.com`)
- `AxonOpsBackup` — Cassandra scheduled snapshot backups with S3, SFTP, and Azure Blob storage support; inline and SecretRef credential patterns

#### Kafka (`kafka.axonops.com`)
- `AxonOpsKafkaTopic` — Kafka topic lifecycle and configuration management
- `AxonOpsKafkaACL` — Kafka ACL entry management
- `AxonOpsKafkaConnector` — Kafka Connect connector management

#### Operator infrastructure
- OpenTelemetry tracing support (OTLP exporter, configurable via environment variables)
- Prometheus metrics endpoint with RBAC protection (port 8443, HTTPS)
- Periodic drift detection on all controllers
- Leader election support for multi-replica deployments
- Namespace-scoped watch (`--watch-namespaces` flag)
- Helm chart published to `oci://ghcr.io/axonops/charts/axonops-operator`
- Kustomize-based installation (`make deploy`)
- Convenience RBAC helper roles (`rbacHelpers.enable=true` in Helm values)

### Security
- All AxonOps API URL path segments escaped with `url.PathEscape()` to prevent path injection
- `errors.As` used throughout for safe API error type discrimination
- Controller metrics served over HTTPS with RBAC-protected scraping endpoint

### Known limitations
- Defaulting webhooks are not yet implemented; all components require explicit configuration (`enabled: true` must be set)
- No conversion webhooks; only `v1alpha1` is available
- External database credentials are not auto-generated; `authentication.secretRef` or inline credentials are required
- `BuildHostURL` hardcodes `https://`, ignoring the `protocol` field in `AxonOpsConnection`
- A new AxonOps API client is created on every reconciliation (no connection reuse)
- Ingress and Gateway resources are not cleaned up automatically when disabled after initial creation

[0.1.0]: https://github.com/axonops/axonops-operator/releases/tag/v0.1.0
