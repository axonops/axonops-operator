#!/usr/bin/env bash
# 01-install-operator.sh — Build operator image and install via Helm.
# When SKIP_CLUSTER_CREATE=true (CI mode): skips build/load; the CI workflow
# has already built the image, pushed it to ttl.sh, and installed the operator.
# The helm upgrade --install below is idempotent and safe to re-run.
set -euo pipefail
source "$(dirname "$0")/lib.sh"

if [[ "${SKIP_CLUSTER_CREATE}" != "true" ]]; then
  # Local / Kind mode: build and load the image.
  echo "Building operator image ${OPERATOR_IMG} ..."
  make docker-build IMG="${OPERATOR_IMG}"

  if kind get clusters 2>/dev/null | grep -q "^${KIND_CLUSTER}$"; then
    echo "Loading image into Kind cluster '${KIND_CLUSTER}' ..."
    kind load docker-image "${OPERATOR_IMG}" --name "${KIND_CLUSTER}"
  else
    echo "Pushing image to ttl.sh (no Kind cluster found) ..."
    docker push "${OPERATOR_IMG}"
  fi
fi

# ---------------------------------------------------------------------------
# Helm install (idempotent — safe to run even if operator is already installed)
# ---------------------------------------------------------------------------
echo "Installing axonops-operator via Helm ..."
helm upgrade --install axonops-operator ./charts/axonops-operator/ \
  --namespace axonops-operator-system \
  --create-namespace \
  --set manager.image.repository="${OPERATOR_IMG%:*}" \
  --set manager.image.tag="${OPERATOR_IMG#*:}" \
  --wait --timeout 120s

echo "Waiting for controller-manager to be Available ..."
kubectl wait deployment/axonops-operator-controller-manager \
  -n axonops-operator-system \
  --for=condition=Available \
  --timeout=120s

pass "Operator is running"
