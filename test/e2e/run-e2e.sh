#!/usr/bin/env bash
# run-e2e.sh — Main E2E runner. Calls scripts 00–99 in order.
# Usage:
#   ./run-e2e.sh          — run all phases
#   ./run-e2e.sh 07       — run a single phase by prefix
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/scripts" && pwd)"
source "${SCRIPT_DIR}/lib.sh"

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

if [[ ${1:-} ]]; then
  SCRIPTS=("$1")
fi

for script in "${SCRIPTS[@]}"; do
  echo ""
  echo "========================================"
  echo "=== Running ${script} ==="
  echo "========================================"
  bash "${SCRIPT_DIR}/${script}.sh"
done

print_summary

if [[ "${SKIP_TEARDOWN:-false}" != "true" ]]; then
  echo ""
  echo "=== Running 99-teardown ==="
  bash "${SCRIPT_DIR}/99-teardown.sh"
fi
