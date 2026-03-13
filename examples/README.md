# AxonOpsMetricAlert Examples

This directory contains example configurations for creating and managing AxonOps metric alerts using the Kubernetes operator.

## Files

- **quickstart.yaml** - Minimal example to get started quickly
- **alerts-complete.yaml** - Comprehensive examples covering various alert scenarios and cluster types

## Quick Start

1. **Set up your credentials:**

```bash
kubectl create secret generic axonops-api-key \
  --from-literal=api_key=YOUR_API_KEY \
  -n default
```

2. **Create an AxonOpsConnection:**

Edit `quickstart.yaml` and replace `YOUR_ORG_ID` with your AxonOps organization ID.

3. **Apply the examples:**

```bash
# Quick start (single alert)
kubectl apply -f quickstart.yaml

# Or apply all examples at once
kubectl apply -f alerts-complete.yaml
```

4. **Verify the alerts:**

```bash
# List all alerts
kubectl get axonopsmetricalerts

# Describe an alert
kubectl describe axonopsmetricalert my-first-alert

# Check alert status
kubectl get axonopsmetricalerts -o wide
```

## Configuration Guide

### AxonOpsConnection

An `AxonOpsConnection` resource holds the credentials and connection details for your AxonOps instance:

```yaml
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsConnection
metadata:
  name: production
spec:
  orgId: my-organization       # Required: your AxonOps org ID
  apiKeyRef:                   # Required: secret reference
    name: axonops-creds        # Secret name
    key: api_key               # Key in secret (default: api_key)

  # Optional: for on-premises deployments
  host: axonops.example.com    # Custom host (default: SaaS)
  protocol: https              # http or https (default: https)
  tokenType: Bearer            # Bearer or AxonApi (default: Bearer)
  tlsSkipVerify: false         # Skip TLS cert verification (default: false)
  useSaml: false               # Enable SAML URL pattern (default: false)
```

### AxonOpsMetricAlert

An `AxonOpsMetricAlert` resource defines a metric alert rule:

```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsMetricAlert
metadata:
  name: high-read-latency
spec:
  # Connection reference (optional if using env vars fallback)
  connectionRef: production

  # Cluster details (required)
  clusterName: prod-cassandra
  clusterType: cassandra       # cassandra, kafka, or dse

  # Alert rule (required)
  name: high-read-latency
  operator: ">"                # >, >=, =, !=, <=, <
  warningValue: 50
  criticalValue: 100
  duration: 15m

  # Dashboard reference (required)
  dashboard: Cassandra Overview
  chart: Read Latency

  # Optional: metric name (auto-derived if not specified)
  metric: cassandra_read_latency_ms

  # Optional: annotations for metadata
  annotations:
    summary: "Cassandra read latency is high"
    description: "Read latency exceeded threshold"

  # Optional: notification integrations
  integrations:
    - type: email
      routing:
        - dba-team@example.com
      overrideWarning: true

  # Optional: filters to narrow alert scope
  filters:
    dc:                        # Data center
      - us-east-1a
    rack:                      # Rack
      - rack1
    hostId:                    # Specific hosts
      - node1
    scope:                     # Alert scope
      - cluster
    keyspace:                  # Cassandra keyspaces
      - system
    consistency:               # Consistency levels
      - LOCAL_QUORUM
    percentile:                # Metrics percentiles
      - 95thPercentile
    topic:                     # Kafka topics
      - events
    groupId:                   # Kafka consumer groups
      - my-consumer-group
    groupBy:                   # Group metrics by dimension
      - dc
      - host_id
```

## Alert States

After creating an alert, check its status:

```bash
kubectl get axonopsmetricalerts
```

**Conditions:**
- `Ready=True` - Alert is synced with AxonOps
- `Ready=False` - Alert failed to sync (check events)

View detailed status:

```bash
kubectl describe axonopsmetricalert my-alert
```

## Environment Variables (Fallback)

If no `connectionRef` is specified, the operator falls back to environment variables:

```bash
export AXONOPS_API_KEY="your-api-key"
export AXONOPS_ORG_ID="your-org-id"
export AXONOPS_HOST="dash.axonops.cloud"        # Optional
export AXONOPS_PROTOCOL="https"                 # Optional
export AXONOPS_TOKEN_TYPE="Bearer"              # Optional
export AXONOPS_TLS_SKIP_VERIFY="false"          # Optional
```

## Common Alert Patterns

### Cassandra Alerts

- **Read/Write Latency** - Monitor response times
- **GC Time** - Garbage collection pauses
- **Compaction Backlog** - Pending compaction tasks
- **Memory Usage** - Heap and off-heap memory
- **Disk Usage** - Storage capacity

### Kafka Alerts

- **Consumer Lag** - Consumers falling behind
- **Replication Factor** - Data redundancy
- **Broker Availability** - Broker health
- **Topic Growth** - Partition size and count

### DSE Alerts

- **Graph Query Latency** - Graph query performance
- **Analytics Processing** - Spark job execution
- **Search Latency** - Solr search performance

## Cleanup

To delete an alert:

```bash
kubectl delete axonopsmetricalert my-alert
```

To delete a connection:

```bash
kubectl delete axonopsconnection production
```

To delete the secret:

```bash
kubectl delete secret axonops-api-key
```

## Troubleshooting

### Alert stuck in "Progressing"

Check logs:
```bash
kubectl logs -n axonops-operator-system deployment/axonops-operator-controller-manager
```

### "AxonOpsConnection not found"

Verify the connection exists in the same namespace:
```bash
kubectl get axonopsconnections
```

### Authentication failed

Verify your API key and org ID:
```bash
kubectl get secret axonops-api-key -o jsonpath='{.data.api_key}' | base64 -d
```

### Dashboard or chart not found

Verify the dashboard and chart names match your AxonOps instance.

## Further Reading

- [AxonOps Documentation](https://docs.axonops.cloud)
- [Kubernetes CRD Documentation](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/)
