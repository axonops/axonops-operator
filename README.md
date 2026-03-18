# AxonOps Kubernetes Operator

<div align=center>
<img src="AxonOps-Operator.png" width="700" height="200">
</div>

A Kubernetes operator that deploys and manages the [AxonOps](https://axonops.com) control plane. It replaces both the AxonOps Helm charts and Terraform provider, giving you a single, declarative interface for running AxonOps entirely within Kubernetes.

## What It Does

- **Deploys the full AxonOps stack** — axon-server, axon-dash, axondb-timeseries, and axondb-search — from a single `AxonOpsServer` custom resource.
- **Manages AxonOps configuration** — alert rules, alert routes, and healthchecks are reconciled as Kubernetes resources and kept in sync with the AxonOps API.
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
| `AxonOpsHealthcheckHTTP` | HTTP endpoint healthchecks |
| `AxonOpsHealthcheckTCP` | TCP port healthchecks |
| `AxonOpsHealthcheckShell` | Shell script healthchecks |

---

## Prerequisites

- Kubernetes 1.28+
- [cert-manager](https://cert-manager.io) — required only when using internal database components (TimeSeries or Search)
- [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api) — required only when using Gateway API ingress

---

## Installation

**Install from Helm Chart:**

```bash
LATEST_VERSION=$(curl -s https://api.github.com/repos/axonops/axonops-operator/releases/latest | grep '"tag_name":' | sed -E 's/.*"v?([^"]+)".*/\1/')
helm upgrade --install -n axonops-operator-system axonops-operator --create-namespace \
  oci://ghcr.io/axonops/charts/axonops-operator:${LATEST_VERSION}
```

**Install from a pre-built YAML bundle:**

```bash
kubectl apply -f https://github.com/axonops/axonops-operator/releases/latest/download/install.yaml
```

---

## Quick Start

### All-in-one deployment (fully managed)

Deploy the complete AxonOps stack with a single resource. The operator provisions all components, generates credentials, and manages TLS certificates automatically.

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsServer
metadata:
  name: axonops
  namespace: axonops
spec:
  server:
    orgName: "my-company"
  timeSeries: {}
  search: {}
  dashboard: {}
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
2. `authentication.username` / `authentication.password` — inline values
3. Auto-generated — operator creates and manages a Secret with random credentials

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

```bash
kubectl delete -k config/samples/  # Remove sample CRs
make uninstall                      # Remove CRDs from the cluster
make undeploy                       # Remove the operator from the cluster
```

---

## License

Copyright 2026.

Licensed under the [Apache License, Version 2.0](LICENSE).
