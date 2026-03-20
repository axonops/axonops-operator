#!/usr/bin/env bash
# 02-deploy-axonops.sh — Deploy AxonOpsServer CR and verify all components come up.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

require_env_vars TEST_NAMESPACE

# Ensure the test namespace exists (skipped when SKIP_CLUSTER_CREATE=true in CI).
kubectl create namespace "${TEST_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

echo "Deploying AxonOpsServer CR in namespace '${TEST_NAMESPACE}' ..."

kubectl apply -f - <<EOF
apiVersion: core.axonops.com/v1alpha1
kind: AxonOpsServer
metadata:
  name: axonops
  namespace: ${TEST_NAMESPACE}
spec:
  server:
    orgName: "${AXONOPS_ORG_ID}"
  timeSeries: {}
  search: {}
  dashboard: {}
EOF

# ---------------------------------------------------------------------------
# Wait for conditions
# ---------------------------------------------------------------------------
echo ""
echo "--- Waiting for CertManagerReady ---"
wait_for_condition axonopsserver/axonops CertManagerReady 120

echo ""
echo "--- Waiting for ServerReady ---"
wait_for_condition axonopsserver/axonops ServerReady 600

echo ""
echo "--- Waiting for DashboardReady ---"
wait_for_condition axonopsserver/axonops DashboardReady 600

# ---------------------------------------------------------------------------
# Assert status fields are populated
# ---------------------------------------------------------------------------
echo ""
echo "--- Asserting status fields ---"
assert_jsonpath_nonempty axonopsserver/axonops '{.status.timeSeriesSecretName}'
assert_jsonpath_nonempty axonopsserver/axonops '{.status.searchSecretName}'
assert_jsonpath_nonempty axonopsserver/axonops '{.status.timeSeriesCertSecretName}'
assert_jsonpath_nonempty axonopsserver/axonops '{.status.searchCertSecretName}'

# ---------------------------------------------------------------------------
# Assert created Kubernetes resources
# ---------------------------------------------------------------------------
echo ""
echo "--- Asserting Kubernetes resources ---"

# StatefulSets
assert_exists statefulset/axon-server
assert_exists statefulset/axondb-timeseries
assert_exists statefulset/axondb-search

# Deployment
assert_exists deployment/axon-dash

# Services (headless + ClusterIP for each component)
assert_exists service/axon-server-agent
assert_exists service/axon-server-api
assert_exists service/axon-server-headless
assert_exists service/axondb-timeseries
assert_exists service/axondb-timeseries-headless
assert_exists service/axondb-search
assert_exists service/axondb-search-headless
assert_exists service/axon-dash

# Auth Secrets
TIMESERIES_SECRET=$(kubectl get axonopsserver/axonops -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.status.timeSeriesSecretName}' 2>/dev/null || true)
SEARCH_SECRET=$(kubectl get axonopsserver/axonops -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.status.searchSecretName}' 2>/dev/null || true)

if [[ -n "${TIMESERIES_SECRET}" ]]; then
  assert_exists "secret/${TIMESERIES_SECRET}"
else
  fail "status.timeSeriesSecretName is empty — cannot verify timeSeries auth Secret"
fi

if [[ -n "${SEARCH_SECRET}" ]]; then
  assert_exists "secret/${SEARCH_SECRET}"
else
  fail "status.searchSecretName is empty — cannot verify search auth Secret"
fi

# TLS Certificate Secrets
TIMESERIES_CERT=$(kubectl get axonopsserver/axonops -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.status.timeSeriesCertSecretName}' 2>/dev/null || true)
SEARCH_CERT=$(kubectl get axonopsserver/axonops -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.status.searchCertSecretName}' 2>/dev/null || true)

if [[ -n "${TIMESERIES_CERT}" ]]; then
  assert_exists "secret/${TIMESERIES_CERT}"
else
  fail "status.timeSeriesCertSecretName is empty — cannot verify timeSeries TLS Secret"
fi

if [[ -n "${SEARCH_CERT}" ]]; then
  assert_exists "secret/${SEARCH_CERT}"
else
  fail "status.searchCertSecretName is empty — cannot verify search TLS Secret"
fi

# ConfigMap for dashboard
assert_exists configmap/axon-dash

# ---------------------------------------------------------------------------
# Assert all StatefulSet/Deployment replicas are ready
# ---------------------------------------------------------------------------
echo ""
echo "--- Asserting replica readiness ---"

for sts in axon-server axondb-timeseries axondb-search; do
  desired=$(kubectl get statefulset/"${sts}" -n "${TEST_NAMESPACE}" \
    -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)
  ready=$(kubectl get statefulset/"${sts}" -n "${TEST_NAMESPACE}" \
    -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
  if [[ "${ready}" == "${desired}" && "${desired}" != "0" ]]; then
    pass "StatefulSet ${sts}: ${ready}/${desired} replicas ready"
  else
    fail "StatefulSet ${sts}: ${ready}/${desired} replicas ready"
  fi
done

desired=$(kubectl get deployment/axon-dash -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.spec.replicas}' 2>/dev/null || echo 0)
ready=$(kubectl get deployment/axon-dash -n "${TEST_NAMESPACE}" \
  -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
if [[ "${ready}" == "${desired}" && "${desired}" != "0" ]]; then
  pass "Deployment axon-dash: ${ready}/${desired} replicas ready"
else
  fail "Deployment axon-dash: ${ready}/${desired} replicas ready"
fi

echo ""
echo "AxonOpsServer deployment complete"
