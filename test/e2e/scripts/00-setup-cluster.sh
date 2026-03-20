#!/usr/bin/env bash
# 00-setup-cluster.sh — Create Kind cluster, install cert-manager and Gateway API CRDs.
# Skipped entirely when SKIP_CLUSTER_CREATE=true (used in CI where Terraform creates k3s).
set -euo pipefail
source "$(dirname "$0")/lib.sh"

if [[ "${SKIP_CLUSTER_CREATE}" == "true" ]]; then
  echo "SKIP_CLUSTER_CREATE=true — skipping cluster setup (cluster already exists)"
  exit 0
fi

# ---------------------------------------------------------------------------
# Kind cluster
# ---------------------------------------------------------------------------
echo "Creating Kind cluster '${KIND_CLUSTER}' ..."
if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
  echo "  Cluster '${KIND_CLUSTER}' already exists, skipping creation"
else
  kind create cluster --name "${KIND_CLUSTER}" --image "${KIND_IMAGE}"
  echo "  Cluster created"
fi

# ---------------------------------------------------------------------------
# cert-manager
# ---------------------------------------------------------------------------
echo "Installing cert-manager ..."
helm upgrade --install cert-manager \
  oci://quay.io/jetstack/charts/cert-manager \
  --version v1.29.0 \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait --timeout 300s
echo "  cert-manager ready"

# ---------------------------------------------------------------------------
# Gateway API (Envoy Gateway)
# ---------------------------------------------------------------------------
echo "Installing Envoy Gateway (Gateway API CRDs) ..."
helm upgrade --install eg \
  oci://docker.io/envoyproxy/gateway-helm \
  --version v1.7.1 \
  --namespace envoy-gateway-system \
  --create-namespace \
  --wait --timeout 180s
echo "  Envoy Gateway ready"

# ---------------------------------------------------------------------------
# Test namespace
# ---------------------------------------------------------------------------
echo "Creating test namespace '${TEST_NAMESPACE}' ..."
kubectl create namespace "${TEST_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
echo "  Namespace ready"
