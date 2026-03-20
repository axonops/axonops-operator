# E2E Testing Plan

End-to-end test suite for the AxonOps Operator. Tests all 18 CRDs against a real Kubernetes cluster with a live AxonOps instance, K8ssandra Cassandra, and Strimzi Kafka.

---

## Goals

1. Install the operator via Helm chart on a fresh cluster
2. Deploy AxonOps stack (`AxonOpsServer`) and verify all components come up
3. Obtain an API token from the live AxonOps instance
4. Deploy a 1-node Cassandra cluster (K8ssandra) and a Kafka cluster (Strimzi)
5. Exercise every CRD: create, verify Ready status, verify API sync, update, delete
6. Verify cleanup: finalizers run, orphaned resources removed

---

## Prerequisites

### Tools

| Tool | Minimum Version | Purpose |
|---|---|---|
| `kind` | 0.20+ | Create local K8s cluster (unless `SKIP_CLUSTER_CREATE=true`) |
| `kubectl` | 1.28+ | Cluster interaction |
| `helm` | 3.14+ | Install operator, cert-manager, K8ssandra, Strimzi |
| `curl` | any | API token creation |
| `jq` | any | JSON parsing |
| `make` | any | Build operator image |
| `docker` | any | Build and load images |

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `SKIP_CLUSTER_CREATE` | `false` | Set `true` to use existing cluster |
| `KIND_CLUSTER` | `axonops-e2e` | Kind cluster name |
| `KIND_IMAGE` | `kindest/node:v1.35.0` | Node image |
| `AXONOPS_HOST` | `sergio.10.16.0.19.nip.io` | AxonOps dashboard URL |
| `AXONOPS_PROTOCOL` | `https` | `http` or `https` |
| `AXONOPS_ORG_ID` | `axonops` | Organization ID for AxonOpsConnection |
| `AXONOPS_CLUSTER_NAME` | `e2e-test-cluster` | Cluster name used in alert CRDs |
| `AXONOPS_KAFKA_CLUSTER` | `e2e-kafka-cluster` | Cluster name used in Kafka CRDs |
| `TEST_NAMESPACE` | `axonops-e2e` | Namespace for test resources |
| `OPERATOR_IMG` | `axonops-operator:e2e` | Operator image to build and load |
| `TIMEOUT` | `300` | Default wait timeout in seconds |
| `K8SSANDRA_NAMESPACE` | `k8ssandra-operator` | Namespace for K8ssandra operator and cluster |
| `STRIMZI_NAMESPACE` | `strimzi` | Namespace for Strimzi operator and Kafka cluster |
| `AXON_AGENT_SERVER_HOST` | `axon-server-agent.$(TEST_NAMESPACE).svc.cluster.local` | Host that Cassandra/Kafka agents use to reach the AxonOps agent port (1888); override for external AxonOps deployments |

---

## Script Layout

```
test/e2e/
├── run-e2e.sh                      # Main runner — calls scripts 00–99 in order
└── scripts/
    ├── lib.sh                       # Shared helpers
    ├── 00-setup-cluster.sh          # Kind cluster + cert-manager + Gateway API
    ├── 01-install-operator.sh       # Helm install operator
    ├── 02-deploy-axonops.sh         # AxonOpsServer CR (full internal stack)
    ├── 03-obtain-token.sh           # API token → Secret → AxonOpsConnection
    ├── 04-install-k8ssandra.sh      # K8ssandra operator + 1-node Cassandra
    ├── 05-install-strimzi.sh        # Strimzi operator + Kafka cluster
    ├── 06-test-core-crds.sh         # AxonOpsServer + AxonOpsConnection
    ├── 07-test-alert-crds.sh        # MetricAlert, LogAlert, AlertRoute, AlertEndpoint
    ├── 08-test-healthcheck-crds.sh  # HealthcheckHTTP, HealthcheckTCP, HealthcheckShell
    ├── 09-test-ops-crds.sh          # DashboardTemplate, AdaptiveRepair, ScheduledRepair, CommitlogArchive
    ├── 10-test-backup-crds.sh       # AxonOpsBackup
    ├── 11-test-kafka-crds.sh        # KafkaTopic, KafkaACL, KafkaConnector
    ├── 12-test-cleanup.sh           # Delete all CRs, verify finalizers and resource removal
    └── 99-teardown.sh               # Uninstall everything, destroy Kind cluster
```

---

## Shared Helpers (`lib.sh`)

```bash
# Wait for a condition on a resource (e.g., Ready=True)
wait_for_condition <resource> <name> <condition> <timeout>

# Assert a kubectl jsonpath equals an expected value
assert_jsonpath <resource> <name> <jsonpath> <expected>

# Assert a Kubernetes resource exists / does not exist
assert_exists <resource> <name>
assert_not_exists <resource> <name>

# Apply YAML from heredoc, wait for Ready, return
apply_and_wait <timeout>  # reads YAML from stdin

# Delete resource and wait for it to be gone (finalizer completion)
delete_and_wait <resource> <name> <timeout>

# Log pass/fail with color output
pass <message>
fail <message>

# Counter for pass/fail summary
TESTS_PASSED=0
TESTS_FAILED=0
print_summary
```

---

## Phase 1 — Infrastructure

### `00-setup-cluster.sh`

1. If `SKIP_CLUSTER_CREATE != true`:
   - `kind create cluster --name $KIND_CLUSTER --image=$KIND_IMAGE`
2. Install cert-manager:
   ```bash
   helm upgrade --install cert-manager oci://quay.io/jetstack/charts/cert-manager \
     --version v1.29.0 --namespace cert-manager --create-namespace \
     --set crds.enabled=true --wait
   ```
3. Install Gateway API CRDs (Envoy Gateway):
   ```bash
   helm install eg oci://docker.io/envoyproxy/gateway-helm \
     --version v1.7.1 -n envoy-gateway-system --create-namespace --wait
   ```
4. Create test namespace: `kubectl create namespace $TEST_NAMESPACE`

### `01-install-operator.sh`

1. Build operator image: `make docker-build IMG=$OPERATOR_IMG`
2. Load into Kind: `kind load docker-image $OPERATOR_IMG --name $KIND_CLUSTER`
  2.1 if not using kind, push the image to `ttl.sh`:
  ```sh
  OPERATOR_IMG=ttl.sh/axonops-operator:${RANDOM_UUID}
  docker push $OPERATOR_IMG
  ```
3. Install via Helm:
   ```bash
   helm upgrade --install axonops-operator ./charts/axonops-operator/ \
     --namespace axonops-operator-system --create-namespace \
     --set manager.image.repository=${OPERATOR_IMG%:*} \
     --set manager.image.tag=${OPERATOR_IMG#*:} \
     --wait --timeout 120s
   ```
4. Verify controller pod is Running:
   ```bash
   kubectl wait --for=condition=Available deployment/axonops-operator-controller-manager \
     -n axonops-operator-system --timeout=120s
   ```

### `02-deploy-axonops.sh`

1. Apply `AxonOpsServer` CR with all internal components enabled:
   ```yaml
   apiVersion: core.axonops.com/v1alpha1
   kind: AxonOpsServer
   metadata:
     name: axonops
     namespace: $TEST_NAMESPACE
   spec:
     server:
       orgName: "$AXONOPS_ORG_ID"
     timeSeries:
       enabled: true
     search:
       enabled: true
     dashboard:
       enabled: true
   ```
2. Wait for status conditions:
   - `CertManagerReady=True` (timeout: 120s)
   - `ServerReady=True` (timeout: 300s)
   - `DashboardReady=True` (timeout: 120s)
3. Verify created resources:
   - 4 StatefulSets (axon-server, axondb-timeseries, axondb-search) + 1 Deployment (axon-dash)
   - Services for each component
   - Auth Secrets for timeSeries and search
   - TLS Certificate Secrets
   - ConfigMap for dashboard

---

## Phase 2 — API Authentication

### `03-obtain-token.sh`

1. Request API token:
   ```bash
   RESPONSE=$(curl -sk -X POST \
     "${AXONOPS_PROTOCOL}://${AXONOPS_HOST}/api/v1/axonops/createApiToken" \
     -H "Content-Type: application/json" \
     -d '{"allowed_roles":["_global_/superuser"],"token_expiry_time":0}')
   API_KEY=$(echo "$RESPONSE" | jq -r '.apiKey')
   ```
2. Create Secret:
   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: axonops-api-key
     namespace: $TEST_NAMESPACE
   stringData:
     api-key: "$API_KEY"
   ```
3. Create AxonOpsConnection:
   ```yaml
   apiVersion: core.axonops.com/v1alpha1
   kind: AxonOpsConnection
   metadata:
     name: e2e-connection
     namespace: $TEST_NAMESPACE
   spec:
     orgId: "$AXONOPS_ORG_ID"
     host: "$AXONOPS_HOST"
     protocol: "$AXONOPS_PROTOCOL"
     tlsSkipVerify: true
     apiKeyRef:
       name: axonops-api-key
       key: api-key
   ```
4. Verify Connection resource exists and Secret is populated.

---

## Phase 3 — Data Platforms

### `04-install-k8ssandra.sh`

1. Install K8ssandra operator:
   ```bash
   helm repo add k8ssandra https://helm.k8ssandra.io/stable
   helm install k8ssandra-operator k8ssandra/k8ssandra-operator \
     -n k8ssandra-operator --create-namespace --wait --version 0.30.0
   ```
2. Apply 1-node K8ssandraCluster CR:

   ```bash
   envsubst < test/e2e/manifests/k8ssandra-cluster.yaml | kubectl apply -f -
   ```

3. Wait for Cassandra pod to be Ready:

   ```bash
   kubectl wait pod -l app.kubernetes.io/created-by=cass-operator --for=condition=Ready \
     -n k8ssandra-operator --timeout=600s
   ```

### `05-install-strimzi.sh`

1. Install Strimzi operator:
   ```bash
   helm repo add strimzi https://strimzi.io/charts/
   helm install strimzi-kafka-operator strimzi/strimzi-kafka-operator \
     -n strimzi --create-namespace --wait --version 0.50.1
   ```
2. Apply Kafka cluster CR:

   ```bash
   envsubst < test/e2e/manifests/kafka-cluster.yaml | kubectl apply -f -
   ```

3. Wait for Kafka pods to be Ready:

   ```bash
   kubectl wait kafka/${AXONOPS_KAFKA_CLUSTER} --for=condition=Ready \
     -n ${STRIMZI_NAMESPACE} --timeout=600s
   ```

---

## Phase 4 — CRD Tests

Every test follows the same pattern:
1. **Create** — apply the CR
2. **Wait** — `wait_for_condition <resource> <name> Ready True $TIMEOUT`
3. **Assert status** — check `syncedAlertID` (or equivalent) is non-empty
4. **Update** — modify a spec field, re-apply
5. **Wait** — confirm `observedGeneration` increments and Ready remains True
6. **Delete** — `delete_and_wait` confirms resource is fully removed

### `06-test-core-crds.sh`

#### AxonOpsServer
Already deployed in Phase 1. Additional assertions:
- `status.timeSeriesSecretName` is non-empty
- `status.searchSecretName` is non-empty
- `status.timeSeriesCertSecretName` is non-empty
- `status.searchCertSecretName` is non-empty
- All 4 StatefulSet/Deployment replicas are Ready
- Services resolve within the cluster

#### AxonOpsConnection
Already created in Phase 2. Additional assertions:
- Resource exists with correct `orgId`
- Referenced Secret exists and has `api-key` data key
- Update: change `tlsSkipVerify` → verify `observedGeneration` increments

### `07-test-alert-crds.sh`

#### AxonOpsMetricAlert
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsMetricAlert
metadata:
  name: io-wait
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: io-wait
  operator: ">="
  warningValue: 20
  criticalValue: 50
  duration: 15m
  dashboard: System
  chart: Avg IO wait CPU per Host
```
- Assert: `status.conditions[Ready]=True`, `status.syncedAlertID` non-empty
- Update: change `criticalValue` to 200, verify re-sync
- Delete: verify finalizer removes alert from AxonOps API

#### AxonOpsLogAlert
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsLogAlert
metadata:
  name: e2e-log-alert
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: e2e-tombstone-alert
  operator: ">="
  warningValue: 1
  criticalValue: 5
  duration: 15m
  level: "error"
  content: "TombstoneOverwhelmingException"
  source: "/var/log/cassandra/system.log"
```
- Assert: `Ready=True`, `syncedAlertID` non-empty
- Update: change `criticalValue`, verify re-sync
- Delete: finalizer cleanup

#### AxonOpsAlertRoute
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsAlertRoute
metadata:
  name: e2e-alert-route
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  integrationName: e2e-slack-channel
  integrationType: slack
  type: metrics
  severity: error
  enableOverride: true
```
- Assert: `Ready=True`, `syncedRouteID` non-empty
- Update: change `severity` to `warning`, verify re-sync
- Delete: finalizer cleanup

#### AxonOpsAlertEndpoint
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsAlertEndpoint
metadata:
  name: e2e-alert-endpoint
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: e2e-webhook-endpoint
  type: webhook
  webhook:
    url: "https://httpbin.org/post"
```
- Assert: `Ready=True`, `syncedEndpointID` non-empty
- Delete: finalizer cleanup

### `08-test-healthcheck-crds.sh`

#### AxonOpsHealthcheckHTTP
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsHealthcheckHTTP
metadata:
  name: e2e-healthcheck-http
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: e2e-http-check
  url: "https://httpbin.org/status/200"
  method: GET
  expectedStatus: 200
  interval: 60s
  timeout: 10s
```
- Assert: `Ready=True`, `syncedHealthcheckID` non-empty
- Delete: finalizer cleanup

#### AxonOpsHealthcheckTCP
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsHealthcheckTCP
metadata:
  name: e2e-healthcheck-tcp
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: e2e-tcp-check
  tcp: "localhost:9042"
  interval: 60s
  timeout: 10s
```
- Assert: `Ready=True`, `syncedHealthcheckID` non-empty
- Delete: finalizer cleanup

#### AxonOpsHealthcheckShell
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsHealthcheckShell
metadata:
  name: e2e-healthcheck-shell
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  name: e2e-shell-check
  script: "echo ok"
  shell: "/bin/bash"
  interval: 60s
  timeout: 10s
```
- Assert: `Ready=True`, `syncedHealthcheckID` non-empty
- Delete: finalizer cleanup

### `09-test-ops-crds.sh`

#### AxonOpsDashboardTemplate
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsDashboardTemplate
metadata:
  name: e2e-dashboard-template
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  dashboardName: "E2E Test Dashboard"
  source:
    inline:
      dashboard:
        filters: []
        panels:
          - title: "Test Panel"
            type: line-chart
            layout: { x: 0, y: 0, w: 12, h: 8 }
            details:
              queries:
                - query: "test_metric"
                  legend: "Test"
```
- Assert: `Ready=True`, `syncedDashboardID` non-empty
- Delete: finalizer cleanup

#### AxonOpsAdaptiveRepair
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsAdaptiveRepair
metadata:
  name: e2e-adaptive-repair
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  active: true
  tableParallelism: 2
  gcGraceThreshold: 172800
```
- Assert: `Ready=True`, `syncedRepairID` non-empty
- Update: change `tableParallelism` to 4, verify re-sync
- Delete: finalizer cleanup

#### AxonOpsScheduledRepair
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsScheduledRepair
metadata:
  name: e2e-scheduled-repair
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  tag: e2e-repair
  scheduleExpression: "0 0 1 * *"
  parallelism: DC-Aware
  segmented: true
  segmentsPerNode: 4
```
- Assert: `Ready=True`, `syncedRepairID` non-empty
- Delete: finalizer cleanup

#### AxonOpsCommitlogArchive
```yaml
apiVersion: alerts.axonops.com/v1alpha1
kind: AxonOpsCommitlogArchive
metadata:
  name: e2e-commitlog-archive
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  remoteType: local
  remotePath: /tmp/commitlogs
  remoteRetention: "7d"
  timeout: "2h"
```
- Assert: `Ready=True`, `syncedArchiveID` non-empty
- Delete: finalizer cleanup

### `10-test-backup-crds.sh`

#### AxonOpsBackup
```yaml
apiVersion: backups.axonops.com/v1alpha1
kind: AxonOpsBackup
metadata:
  name: e2e-backup
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_CLUSTER_NAME
  clusterType: cassandra
  tag: e2e-daily-backup
  schedule: true
  scheduleExpression: "0 1 * * *"
  localRetention: "7d"
  timeout: "4h"
```
- Assert: `Ready=True`, `syncedBackupID` non-empty
- Update: change `localRetention` to `14d`, verify re-sync
- Delete: finalizer cleanup

### `11-test-kafka-crds.sh`

#### AxonOpsKafkaTopic
```yaml
apiVersion: kafka.axonops.com/v1alpha1
kind: AxonOpsKafkaTopic
metadata:
  name: e2e-kafka-topic
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_KAFKA_CLUSTER
  name: e2e-test-topic
  partitions: 3
  replicationFactor: 1
  config:
    cleanup.policy: "delete"
    retention.ms: "86400000"
```
- Assert: `Ready=True`, `syncedTopicID` non-empty
- Update: change `partitions` to 6, verify re-sync
- Delete: finalizer cleanup

#### AxonOpsKafkaACL
```yaml
apiVersion: kafka.axonops.com/v1alpha1
kind: AxonOpsKafkaACL
metadata:
  name: e2e-kafka-acl
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_KAFKA_CLUSTER
  resourceType: TOPIC
  resourceName: e2e-test-topic
  resourcePatternType: LITERAL
  principal: "User:e2e-producer"
  host: "*"
  operation: WRITE
  permissionType: ALLOW
```
- Assert: `Ready=True`, `syncedACLID` non-empty
- Delete: finalizer cleanup

#### AxonOpsKafkaConnector
```yaml
apiVersion: kafka.axonops.com/v1alpha1
kind: AxonOpsKafkaConnector
metadata:
  name: e2e-kafka-connector
  namespace: $TEST_NAMESPACE
spec:
  connectionRef: e2e-connection
  clusterName: $AXONOPS_KAFKA_CLUSTER
  connectClusterName: e2e-connect
  name: e2e-file-sink
  config:
    connector.class: "org.apache.kafka.connect.file.FileStreamSinkConnector"
    topics: "e2e-test-topic"
    file: "/tmp/e2e-output.txt"
    tasks.max: "1"
```
- Assert: `Ready=True`, `syncedConnectorID` non-empty
- Delete: finalizer cleanup

---

## Phase 5 — Cleanup Tests

### `12-test-cleanup.sh`

Tests that deleting CRs triggers proper finalizer execution and resource cleanup.

1. **Delete all alert/ops CRs** created in Phase 4
   - For each: `delete_and_wait` — confirm the CR is fully removed
   - Verify synced resources are removed from AxonOps API (optional: curl the API to confirm)

2. **Delete AxonOpsConnection**
   - Verify Secret still exists (not owned by connection)
   - Verify connection CR is gone

3. **Disable AxonOpsServer components one by one**
   - Patch `dashboard.enabled: false` → verify Deployment, Service, ConfigMap deleted
   - Patch `search.enabled: false` → verify StatefulSet, Services, Secrets, TLS cert deleted
   - Patch `timeSeries.enabled: false` → verify StatefulSet, Services, Secrets, TLS cert deleted

4. **Delete AxonOpsServer**
   - Verify all remaining resources cleaned up (server StatefulSet, Services, etc.)
   - Verify namespace has no orphaned resources with `app.kubernetes.io/managed-by: axonops-operator`

---

## Phase 6 — Teardown

### `99-teardown.sh`

1. Uninstall operator: `helm uninstall axonops-operator -n axonops-operator-system`
2. Uninstall K8ssandra: `helm uninstall k8ssandra-operator -n k8ssandra-operator`
3. Uninstall Strimzi: `helm uninstall strimzi-kafka-operator -n strimzi`
4. Uninstall cert-manager: `helm uninstall cert-manager -n cert-manager`
5. Delete namespaces
6. If `SKIP_CLUSTER_CREATE != true`: `kind delete cluster --name $KIND_CLUSTER`

---

## Runner Script (`run-e2e.sh`)

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/scripts" && pwd)"
source "$SCRIPT_DIR/lib.sh"

SCRIPTS=(
  00-setup-cluster
  01-install-operator
  02-deploy-axonops
  03-obtain-token
  04-install-k8ssandra
  05-install-strimzi
  06-test-core-crds
  07-test-alert-crds
  08-test-healthcheck-crds
  09-test-ops-crds
  10-test-backup-crds
  11-test-kafka-crds
  12-test-cleanup
)

# Allow running a single phase: ./run-e2e.sh 07
if [[ ${1:-} ]]; then
  SCRIPTS=("$1")
fi

for script in "${SCRIPTS[@]}"; do
  echo "=== Running $script ==="
  bash "$SCRIPT_DIR/${script}.sh"
done

print_summary

# Always teardown unless SKIP_TEARDOWN=true
if [[ "${SKIP_TEARDOWN:-false}" != "true" ]]; then
  bash "$SCRIPT_DIR/99-teardown.sh"
fi
```

---

## CRD Test Matrix Summary

| # | CRD | Script | Create | Wait Condition | Status Field | Update | Delete |
|---|---|---|---|---|---|---|---|
| 1 | AxonOpsServer | 06 | Phase 1 | ServerReady=True | timeSeriesSecretName | n/a (tested in 12) | 12 |
| 2 | AxonOpsConnection | 06 | Phase 2 | exists | orgId | tlsSkipVerify | 12 |
| 3 | AxonOpsMetricAlert | 07 | YAML | Ready=True | syncedAlertID | criticalValue | 07+12 |
| 4 | AxonOpsLogAlert | 07 | YAML | Ready=True | syncedAlertID | criticalValue | 07+12 |
| 5 | AxonOpsAlertRoute | 07 | YAML | Ready=True | syncedRouteID | severity | 07+12 |
| 6 | AxonOpsAlertEndpoint | 07 | YAML | Ready=True | syncedEndpointID | — | 07+12 |
| 7 | AxonOpsHealthcheckHTTP | 08 | YAML | Ready=True | syncedHealthcheckID | — | 08+12 |
| 8 | AxonOpsHealthcheckTCP | 08 | YAML | Ready=True | syncedHealthcheckID | — | 08+12 |
| 9 | AxonOpsHealthcheckShell | 08 | YAML | Ready=True | syncedHealthcheckID | — | 08+12 |
| 10 | AxonOpsDashboardTemplate | 09 | YAML | Ready=True | syncedDashboardID | — | 09+12 |
| 11 | AxonOpsAdaptiveRepair | 09 | YAML | Ready=True | syncedRepairID | tableParallelism | 09+12 |
| 12 | AxonOpsScheduledRepair | 09 | YAML | Ready=True | syncedRepairID | — | 09+12 |
| 13 | AxonOpsCommitlogArchive | 09 | YAML | Ready=True | syncedArchiveID | — | 09+12 |
| 14 | AxonOpsBackup | 10 | YAML | Ready=True | syncedBackupID | localRetention | 10+12 |
| 15 | AxonOpsKafkaTopic | 11 | YAML | Ready=True | syncedTopicID | partitions | 11+12 |
| 16 | AxonOpsKafkaACL | 11 | YAML | Ready=True | syncedACLID | — | 11+12 |
| 17 | AxonOpsKafkaConnector | 11 | YAML | Ready=True | syncedConnectorID | — | 11+12 |
| 18 | AxonOpsServer (external) | 06 | optional | ServerReady=True | — | — | — |

---

## Manifests Directory

User-provided manifests go under `test/e2e/manifests/`:

```
test/e2e/manifests/
├── k8ssandra-cluster.yaml     # 1-node K8ssandraCluster (user provides)
├── kafka-cluster.yaml         # Strimzi Kafka cluster (user provides)
└── (generated at runtime by scripts)
```

---

## CI Integration

### Makefile Target

```makefile
.PHONY: test-e2e-full
test-e2e-full:
	bash test/e2e/run-e2e.sh
```

### GitHub Actions Workflow (`.github/workflows/test-e2e-full.yml`)

```yaml
name: E2E Full Suite
on:
  workflow_dispatch:    # manual trigger only — requires live AxonOps
  # schedule:
  #   - cron: '0 2 * * 1'  # optional: weekly Monday 2am

jobs:
  e2e:
    runs-on: ubuntu-latest
    env:
      AXONOPS_HOST: ${{ secrets.AXONOPS_HOST }}
      AXONOPS_ORG_ID: ${{ secrets.AXONOPS_ORG_ID }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - name: Install Kind
        run: go install sigs.k8s.io/kind@latest
      - name: Run E2E Suite
        run: make test-e2e-full
      - name: Collect logs on failure
        if: failure()
        run: |
          kubectl logs -n axonops-operator-system deployment/axonops-operator-controller-manager -c manager --tail=200 || true
          kubectl get events -n $TEST_NAMESPACE --sort-by='.lastTimestamp' || true
```

---

## Estimated Timing

| Phase | Scripts | Estimated Duration |
|---|---|---|
| Infrastructure | 00–02 | ~5–8 min (cert-manager + operator + AxonOps stack) |
| Auth | 03 | ~10 sec |
| Data platforms | 04–05 | ~5–10 min (Cassandra + Kafka startup) |
| CRD tests | 06–11 | ~5–10 min (18 CRDs, mostly API sync waits) |
| Cleanup tests | 12 | ~2–3 min |
| Teardown | 99 | ~1 min |
| **Total** | | **~20–30 min** |
