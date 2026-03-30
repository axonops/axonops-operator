# axonops-operator

[![Artifact Hub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/axonops-operator)](https://artifacthub.io/packages/helm/axonops-operator/axonops-operator)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](https://github.com/axonops/axonops-operator/blob/main/LICENSE)
[![Operator Capabilities: Full Lifecycle](https://img.shields.io/badge/Operator%20Capabilities-Full%20Lifecycle-green)](https://artifacthub.io/packages/helm/axonops-operator/axonops-operator)

A Kubernetes operator that manages the full AxonOps observability stack as declarative custom resources. It covers infrastructure (server deployment, storage, dashboard), operational configuration (alert rules, healthchecks, backups, repairs), and Kafka resource management — replacing manual Helm chart overrides, Terraform provider calls, and Ansible playbooks with GitOps-ready CRDs.

## Prerequisites

| Requirement | Version | Notes |
|---|---|---|
| Kubernetes | 1.28+ | |
| Helm | 3.x | |
| cert-manager | any | Required only when deploying internal TimeSeries or Search components via `AxonOpsPlatform` |
| Gateway API CRDs | any | Required only when using Gateway API ingress in `AxonOpsPlatform` |

## Installation

Install from the OCI registry:

```bash
helm upgrade --install axonops-operator \
  oci://ghcr.io/axonops/charts/axonops-operator \
  --namespace axonops-operator-system \
  --create-namespace
```

The chart installs CRDs by default (`crd.enable=true`). CRDs are preserved on uninstall by default (`crd.keep=true`) to prevent accidental data loss.

### Verifying the installation

```bash
kubectl -n axonops-operator-system get deployment
kubectl get crd | grep axonops.com
```

## Custom Resource Definitions

The operator introduces 19 CRDs across four API groups.

### `core.axonops.com/v1alpha1`

| CRD | Description |
|---|---|
| `AxonOpsPlatform` | Deploys the AxonOps stack: axon-server, axon-dash, axondb-timeseries, axondb-search |
| `AxonOpsConnection` | Shared API authentication configuration for all other CRDs |

### `alerts.axonops.com/v1alpha1`

| CRD | Description |
|---|---|
| `AxonOpsMetricAlert` | Metric threshold alert rules |
| `AxonOpsLogAlert` | Log pattern alert rules |
| `AxonOpsAlertRoute` | Routes alerts to integration endpoints |
| `AxonOpsAlertEndpoint` | Integration endpoint definitions (PagerDuty, Slack, etc.) |
| `AxonOpsHealthcheckHTTP` | HTTP endpoint healthchecks |
| `AxonOpsHealthcheckTCP` | TCP port healthchecks |
| `AxonOpsHealthcheckShell` | Shell script healthchecks |
| `AxonOpsDashboardTemplate` | Declarative dashboard management |
| `AxonOpsAdaptiveRepair` | Adaptive Cassandra repair configuration |
| `AxonOpsScheduledRepair` | Cron-based Cassandra scheduled repairs |
| `AxonOpsCommitlogArchive` | Commitlog archive settings |
| `AxonOpsSilenceWindow` | Alert silence windows |
| `AxonOpsLogCollector` | Log collector configuration |

### `backups.axonops.com/v1alpha1`

| CRD | Description |
|---|---|
| `AxonOpsBackup` | Cassandra scheduled snapshot backups (S3, SFTP, Azure Blob) |

### `kafka.axonops.com/v1alpha1`

| CRD | Description |
|---|---|
| `AxonOpsKafkaTopic` | Kafka topic lifecycle and configuration management |
| `AxonOpsKafkaACL` | Kafka ACL entry management |
| `AxonOpsKafkaConnector` | Kafka Connect connector management |

## Quick Start

### Deploy the AxonOps stack (fully managed components)

Apply this resource after installing the operator. cert-manager MUST be installed in the cluster when `timeSeries.enabled` or `search.enabled` is `true`.

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsPlatform
metadata:
  name: axonops
  namespace: axonops
spec:
  server:
    orgName: "my-company"
  timeSeries:
    enabled: true
  search:
    enabled: true
  dashboard:
    enabled: true
```

### Deploy the AxonOps stack with external databases

Use this form when Cassandra and Elasticsearch are managed outside the operator.

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsPlatform
metadata:
  name: axonops
  namespace: axonops
spec:
  server:
    orgName: "my-company"
  timeSeries:
    external:
      hosts:
        - cassandra-node1.example.com:9042
        - cassandra-node2.example.com:9042
    authentication:
      secretRef: cassandra-credentials
  search:
    external:
      hosts:
        - https://elasticsearch.example.com:9200
    authentication:
      secretRef: elasticsearch-credentials
  dashboard: {}
```

### Configure an API connection and alert rule

`AxonOpsConnection` provides credentials for all other CRDs in the same namespace. `tokenType` is auto-detected from the host: cloud hosts (`dash.axonops.cloud`, `dash.axonopsdev.com`) default to `Bearer`; self-hosted instances default to `AxonApi`. Set it explicitly to override.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: axonops-api-key
  namespace: axonops
stringData:
  api-key: "your-api-key-here"
---
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsConnection
metadata:
  name: axonops-api
  namespace: axonops
spec:
  orgId: "my-org-id"
  host: "axonops.example.com"   # omit for dash.axonops.cloud
  apiKeyRef:
    name: axonops-api-key
    key: api-key
---
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsMetricAlert
metadata:
  name: high-read-latency
  namespace: axonops
spec:
  connectionRef: axonops-api
  clusterName: production-cluster
  clusterType: cassandra
  name: high-read-latency
  operator: ">"
  warningValue: 50
  criticalValue: 100
  duration: 15m
  dashboard: Cassandra Overview
  chart: Read Latency
```

### Configure a scheduled backup

```yaml
apiVersion: backups.axonops.com/v1alpha1
kind: AxonOpsBackup
metadata:
  name: daily-backup
  namespace: axonops
spec:
  connectionRef: axonops-api
  clusterName: production-cluster
  schedule: "0 2 * * *"
  storageType: s3
  s3:
    bucket: my-backup-bucket
    region: us-east-1
    credentialsRef:
      name: s3-credentials
```

## Configuration

### Parameters

| Parameter | Description | Default |
|---|---|---|
| `nameOverride` | Partial override for the chart name used in resource names | `""` |
| `fullnameOverride` | Full override for the chart name used in resource names | `""` |
| `manager.replicas` | Number of controller manager replicas. Set to `>1` only with `--leader-elect` in `manager.args` | `1` |
| `manager.image.repository` | Controller manager container image repository | `ghcr.io/axonops/axonops-operator` |
| `manager.image.tag` | Image tag. Defaults to `appVersion` from `Chart.yaml` when empty | `""` |
| `manager.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `manager.args` | Arguments passed to the manager binary. See [Manager flags](#manager-flags) | `["--leader-elect"]` |
| `manager.env` | Additional environment variables for the manager container (list of `name`/`value` objects). See [Environment variables](#environment-variables) | `[]` |
| `manager.envOverrides` | Environment variables as a map (`VAR: value`). Takes precedence over entries in `manager.env` with the same name | `{}` |
| `manager.imagePullSecrets` | Image pull secrets for the manager pod | `[]` |
| `manager.podSecurityContext.runAsNonRoot` | Require the pod to run as a non-root user | `true` |
| `manager.podSecurityContext.seccompProfile.type` | Seccomp profile type | `RuntimeDefault` |
| `manager.securityContext.allowPrivilegeEscalation` | Allow privilege escalation in the manager container | `false` |
| `manager.securityContext.capabilities.drop` | Linux capabilities to drop | `["ALL"]` |
| `manager.securityContext.readOnlyRootFilesystem` | Mount the root filesystem read-only | `true` |
| `manager.resources.limits.cpu` | CPU limit for the manager container | `500m` |
| `manager.resources.limits.memory` | Memory limit for the manager container | `128Mi` |
| `manager.resources.requests.cpu` | CPU request for the manager container | `10m` |
| `manager.resources.requests.memory` | Memory request for the manager container | `64Mi` |
| `manager.affinity` | Affinity rules for the manager pod | `{}` |
| `manager.nodeSelector` | Node selector for the manager pod | `{}` |
| `manager.tolerations` | Tolerations for the manager pod | `[]` |
| `rbacHelpers.enable` | Install convenience `admin`, `editor`, and `viewer` ClusterRoles for each CRD | `false` |
| `crd.enable` | Install CRDs as part of the Helm release | `true` |
| `crd.keep` | Retain CRDs when the Helm release is uninstalled. MUST remain `true` in production to prevent data loss | `true` |
| `metrics.enable` | Expose the `/metrics` endpoint with RBAC protection | `true` |
| `metrics.port` | Port the metrics server binds to | `8443` |
| `certManager.enable` | Enable cert-manager integration for TLS certificates on the metrics endpoint and webhooks | `false` |
| `prometheus.enable` | Install a Prometheus `ServiceMonitor` for metrics scraping. Requires the Prometheus Operator | `false` |

### Manager flags

Pass additional flags via `manager.args`. The following flags are supported:

| Flag | Description | Default |
|---|---|---|
| `--leader-elect` | Enable leader election. MUST be set when `manager.replicas > 1` | enabled by default |
| `--metrics-bind-address=:8443` | Address the metrics endpoint binds to. Use `:8080` for HTTP, `0` to disable | `:8443` (HTTPS) |
| `--metrics-secure=true` | Serve metrics over HTTPS | `true` |
| `--metrics-cert-path=<dir>` | Directory containing the metrics server TLS certificate | — |
| `--metrics-cert-name=tls.crt` | Metrics server certificate filename | `tls.crt` |
| `--metrics-cert-key=tls.key` | Metrics server key filename | `tls.key` |
| `--health-probe-bind-address=:8081` | Address the liveness/readiness probe endpoint binds to | `:8081` |
| `--webhook-cert-path=<dir>` | Directory containing the webhook TLS certificate | — |
| `--webhook-cert-name=tls.crt` | Webhook certificate filename | `tls.crt` |
| `--webhook-cert-key=tls.key` | Webhook key filename | `tls.key` |
| `--cluster-issuer=<name>` | cert-manager `ClusterIssuer` name for TLS certificates | `axonops-selfsigned` |
| `--enable-http2` | Enable HTTP/2 for the metrics and webhook servers. Disabled by default as a security hardening measure | `false` |
| `--watch-namespaces=ns1,ns2` | Comma-separated list of namespaces to watch. Omit to watch all namespaces | all namespaces |
| `--zap-log-level=info` | Log level: `debug`, `info`, `error`, or an integer verbosity value | `info` |
| `--zap-encoder=json` | Log format: `json` or `console` | `json` |
| `--zap-devel` | Enable development mode logging (more verbose, non-JSON output) | `false` |

### Environment variables

Configure via `manager.env` (list) or `manager.envOverrides` (map). Values in `manager.envOverrides` take precedence over `manager.env` entries with the same name.

| Variable | Description | Default |
|---|---|---|
| `DISABLE_METRICS` | Set to `true` to disable the Prometheus metrics endpoint entirely | unset |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint to enable OpenTelemetry tracing (e.g. `http://otel-collector:4317`). Tracing is disabled when unset | unset |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP exporter protocol: `grpc` or `http/protobuf` | `grpc` |
| `OTEL_EXPORTER_OTLP_INSECURE` | Set to `true` to disable TLS for the OTLP exporter (useful for in-cluster collectors) | unset |
| `OTEL_EXPORTER_OTLP_HEADERS` | Additional headers sent with every OTLP request, as `key=value` pairs separated by commas | unset |

#### Example: enable OpenTelemetry tracing

```yaml
manager:
  env:
    - name: OTEL_EXPORTER_OTLP_ENDPOINT
      value: "http://otel-collector.monitoring:4317"
    - name: OTEL_EXPORTER_OTLP_INSECURE
      value: "true"
```

### Prometheus metrics

When `metrics.enable=true` (the default), the manager exposes a Prometheus-compatible `/metrics` endpoint on port `8443` over HTTPS. The endpoint is protected by Kubernetes RBAC — the scraping ServiceAccount MUST have the `metrics-reader` ClusterRole bound to it.

Enable the `ServiceMonitor` for the Prometheus Operator:

```bash
helm upgrade --install axonops-operator \
  oci://ghcr.io/axonops/charts/axonops-operator \
  --namespace axonops-operator-system \
  --set prometheus.enable=true \
  --set certManager.enable=true
```

> **Note:** When `certManager.enable=false`, the `ServiceMonitor` configures `insecureSkipVerify: true` for TLS. Enable cert-manager in production to avoid this.

### Multi-replica deployment

Set `manager.replicas` greater than `1` only when `--leader-elect` is present in `manager.args` (it is included by default). Without leader election, multiple replicas produce duplicate reconciliations and conflicting writes.

```yaml
manager:
  replicas: 2
  args:
    - --leader-elect
```

### Namespace-scoped watch

By default the operator watches all namespaces. To restrict it to specific namespaces:

```yaml
manager:
  args:
    - --leader-elect
    - --watch-namespaces=axonops,monitoring
```

## Uninstalling

```bash
helm uninstall axonops-operator -n axonops-operator-system
```

CRDs are retained by default (`crd.keep=true`). This preserves all existing `AxonOpsPlatform`, `AxonOpsBackup`, and other custom resources in the cluster. To remove CRDs and all associated resources after uninstalling the chart:

```bash
kubectl get crd -o name | grep axonops.com | xargs kubectl delete
```

> **Warning:** Deleting CRDs removes all custom resources of those types from the cluster. This action is irreversible. Ensure all resources are backed up or no longer needed before proceeding.

## Source and support

- Source code: [github.com/axonops/axonops-operator](https://github.com/axonops/axonops-operator)
- Documentation: [docs.axonops.com](https://docs.axonops.com)
- Container image: [ghcr.io/axonops/axonops-operator](https://github.com/axonops/axonops-operator/pkgs/container/axonops-operator)
- Support: [support@axonops.com](mailto:support@axonops.com)
