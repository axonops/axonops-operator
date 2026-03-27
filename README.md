# AxonOps Kubernetes Operator

<div align=center>
<img src="AxonOps-Operator.png" width="700" height="200">
</div>

A Kubernetes operator that deploys and manages the [AxonOps](https://axonops.com) control plane. It replaces both the AxonOps Helm charts and Terraform provider, giving you a single, declarative interface for running AxonOps entirely within Kubernetes.

## What It Does

- **Deploys the full AxonOps stack** — axon-server, axon-dash, axondb-timeseries, and axondb-search — from a single `AxonOpsServer` custom resource.
- **Manages AxonOps configuration** — alert rules, alert routes, healthchecks, dashboard templates, backups, scheduled repairs, and commitlog archives are reconciled as Kubernetes resources and kept in sync with the AxonOps API.
- **Manages Kafka resources** — topics, ACLs, and connectors for Kafka-based clusters.
- **Handles day-2 operations** — credential rotation, TLS certificate management, startup ordering, and Ingress/Gateway API configuration.

---

## CRDs

### `core.axonops.com/v1alpha1`

| Kind | Purpose |
|---|---|
| `AxonOpsServer` | Deploys and manages the full AxonOps server stack |
| `AxonOpsConnection` | Stores reusable API credentials for the AxonOps API |

### `alerts.axonops.com/v1alpha1`

| Kind | Purpose |
|---|---|
| `AxonOpsMetricAlert` | Metric threshold alerts |
| `AxonOpsLogAlert` | Log pattern alerts |
| `AxonOpsAlertRoute` | Alert routing and notification channels |
| `AxonOpsAlertEndpoint` | Alert notification endpoints (email, Slack, PagerDuty, etc.) |
| `AxonOpsHealthcheckHTTP` | HTTP endpoint healthchecks |
| `AxonOpsHealthcheckTCP` | TCP port healthchecks |
| `AxonOpsHealthcheckShell` | Shell script healthchecks |
| `AxonOpsDashboardTemplate` | Declarative dashboard management |
| `AxonOpsAdaptiveRepair` | Adaptive repair scheduling |
| `AxonOpsScheduledRepair` | Scheduled repair management |
| `AxonOpsCommitlogArchive` | Commitlog archive management |

### `backups.axonops.com/v1alpha1`

| Kind | Purpose |
|---|---|
| `AxonOpsBackup` | Backup scheduling and management |

### `kafka.axonops.com/v1alpha1`

| Kind | Purpose |
|---|---|
| `AxonOpsKafkaTopic` | Kafka topic management |
| `AxonOpsKafkaACL` | Kafka ACL management |
| `AxonOpsKafkaConnector` | Kafka connector management |

---

## Prerequisites

- Kubernetes 1.28+
- [cert-manager](https://cert-manager.io) — required only when using internal database components (TimeSeries or Search)
- [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) — required only when using Gateway API ingress

---

## Installation

**Install from OCI registry (recommended):**

The Helm chart is published as an OCI artifact on GitHub Container Registry. Install with:

```bash
helm upgrade --install axonops-operator \
  oci://ghcr.io/axonops/charts/axonops-operator:0.0.3 \
  --namespace axonops-operator-system --create-namespace
```

To see available versions, check the [releases page](https://github.com/axonops/axonops-operator/releases) or the [package registry](https://github.com/axonops/axonops-operator/pkgs/container/charts%2Faxonops-operator).

**Install from local chart source:**

```bash
helm upgrade --install axonops-operator \
  ./charts/axonops-operator/ \
  --namespace axonops-operator-system --create-namespace
```

**Install from source with Kustomize:**

```bash
make deploy IMG=<registry>/<project>:<tag>
```

---

## Quick Start

### All-in-one deployment (fully managed)

Deploy the complete AxonOps stack with a single resource. The operator provisions all components, generates credentials, and manages TLS certificates automatically.

> **Note:** All components must have `enabled: true` set explicitly until the defaulting webhook is implemented.

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsServer
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

### External databases

Connect the AxonOps server to existing Cassandra and Elasticsearch/OpenSearch clusters instead of running them in-cluster.

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsServer
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
      tls:
        enabled: false
    authentication:
      secretRef: cassandra-credentials   # Secret with AXONOPS_DB_USER / AXONOPS_DB_PASSWORD
  search:
    external:
      hosts:
        - https://elasticsearch.example.com:9200
    authentication:
      secretRef: elasticsearch-credentials  # Secret with AXONOPS_SEARCH_USER / AXONOPS_SEARCH_PASSWORD
  dashboard: {}
```

### Alert management

Create an `AxonOpsConnection` once per namespace, then reference it from alert resources.

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
  host: "axonops.example.com"
  protocol: "https"
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
  annotations:
    summary: "Cassandra read latency is high"
```

---

## AxonOpsServer Components

Each component can operate in **internal** (operator-managed) or **external** (user-provided) mode.

| Component | Image | Internal | External |
|---|---|---|---|
| `axon-server` | `axon-server` | StatefulSet | n/a |
| `axon-dash` | `axon-dash` | Deployment | n/a |
| `axondb-timeseries` | `axondb-timeseries` | StatefulSet | Cassandra-compatible |
| `axondb-search` | `axondb-search` | StatefulSet | Elasticsearch/OpenSearch |

### Authentication

For each database component, the operator uses credentials in this priority order:

1. `authentication.secretRef` — reference an existing Secret
2. Auto-generated — operator creates and manages a Secret with random credentials

> **Note:** Inline `authentication.username` / `authentication.password` fields are not yet supported. Use `secretRef` or rely on auto-generation.

### Ingress and Gateway API

Both Dashboard and Server endpoints (agent and API) support `ingress` and `gateway` configuration independently. You can enable one, both, or neither per endpoint.

### TLS

When using internal TimeSeries or Search components, the operator creates TLS certificates via cert-manager. cert-manager is not required for external database configurations.

### Startup ordering

The operator enforces dependency ordering: Server waits for its databases to be ready; Dashboard waits for Server.

---

## Samples

Pre-built sample resources are available under `config/samples/`:

```bash
kubectl apply -k config/samples/
```

For more detailed examples including alert configuration, K8ssandra integration, and full-stack deployments, see the [`examples/`](examples/) directory.

---

## Development

```bash
make manifests        # Regenerate CRDs and RBAC from kubebuilder markers
make generate         # Regenerate DeepCopy methods
make fmt && make vet  # Format and vet code
make lint             # Run linter
make test             # Run unit tests (uses envtest)
make build            # Build the manager binary
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full development workflow.

---

## Uninstall

**If installed via Helm:**

```bash
kubectl delete -k config/samples/                                         # Remove sample CRs
helm uninstall axonops-operator -n axonops-operator-system                # Remove the operator
```

**If installed via Kustomize / Make:**

```bash
kubectl delete -k config/samples/  # Remove sample CRs
make uninstall                      # Remove CRDs from the cluster
make undeploy                       # Remove the operator from the cluster
```

---

## License

Copyright 2026 AxonOps Ltd.

Licensed under the [Apache License, Version 2.0](LICENSE).
