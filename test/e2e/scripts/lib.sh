#!/usr/bin/env bash
# lib.sh — Shared helpers for all E2E scripts.
# Source this file at the top of every script: source "$(dirname "$0")/lib.sh"
set -euo pipefail

# ---------------------------------------------------------------------------
# Environment defaults
# ---------------------------------------------------------------------------
export SKIP_CLUSTER_CREATE="${SKIP_CLUSTER_CREATE:-false}"
export SKIP_TEARDOWN="${SKIP_TEARDOWN:-false}"
export KIND_CLUSTER="${KIND_CLUSTER:-axonops-e2e}"
export KIND_IMAGE="${KIND_IMAGE:-kindest/node:v1.35.0}"
export AXONOPS_HOST="${AXONOPS_HOST:-}"
export AXONOPS_PROTOCOL="${AXONOPS_PROTOCOL:-https}"
export AXONOPS_ORG_ID="${AXONOPS_ORG_ID:-axonops}"
export AXONOPS_CLUSTER_NAME="${AXONOPS_CLUSTER_NAME:-e2e-test-cluster}"
export AXONOPS_KAFKA_CLUSTER="${AXONOPS_KAFKA_CLUSTER:-e2e-kafka-cluster}"
export TEST_NAMESPACE="${TEST_NAMESPACE:-axonops-e2e}"
export OPERATOR_IMG="${OPERATOR_IMG:-axonops-operator:e2e}"
export TIMEOUT="${TIMEOUT:-300}"
export K8SSANDRA_NAMESPACE="${K8SSANDRA_NAMESPACE:-k8ssandra-operator}"
export STRIMZI_NAMESPACE="${STRIMZI_NAMESPACE:-strimzi}"
# Host that Cassandra/Kafka agents use to reach AxonOps agent port (1888).
# Defaults to the in-cluster service for the TEST_NAMESPACE.
export AXON_AGENT_SERVER_HOST="${AXON_AGENT_SERVER_HOST:-axon-server-agent.${TEST_NAMESPACE}.svc.cluster.local}"

# ---------------------------------------------------------------------------
# Counters
# ---------------------------------------------------------------------------
TESTS_PASSED=0
TESTS_FAILED=0

# ---------------------------------------------------------------------------
# Colour helpers
# ---------------------------------------------------------------------------
_green() { printf '\033[0;32m%s\033[0m\n' "$*"; }
_red()   { printf '\033[0;31m%s\033[0m\n' "$*"; }
_yellow(){ printf '\033[0;33m%s\033[0m\n' "$*"; }

pass() {
  TESTS_PASSED=$(( TESTS_PASSED + 1 ))
  _green "  PASS: $*"
}

fail() {
  TESTS_FAILED=$(( TESTS_FAILED + 1 ))
  _red   "  FAIL: $*"
}

print_summary() {
  echo ""
  echo "========================================"
  echo "  E2E Summary"
  echo "========================================"
  _green "  Passed: ${TESTS_PASSED}"
  if [[ "${TESTS_FAILED}" -gt 0 ]]; then
    _red "  Failed: ${TESTS_FAILED}"
    return 1
  fi
  _green "  Failed: 0"
}

# ---------------------------------------------------------------------------
# wait_for_condition <resource/name> <condition> <timeout_seconds> [namespace]
#
# Waits for a Kubernetes resource condition to become True.
# Example: wait_for_condition axonopsplatform/axonops ServerReady 300
# ---------------------------------------------------------------------------
wait_for_condition() {
  local resource="$1"
  local condition="$2"
  local timeout="${3:-${TIMEOUT}}"
  local ns="${4:-${TEST_NAMESPACE}}"

  echo "  Waiting for ${resource} condition ${condition}=True (timeout: ${timeout}s) ..."
  if kubectl wait "${resource}" \
      --for="condition=${condition}=True" \
      -n "${ns}" \
      --timeout="${timeout}s"; then
    pass "${resource} ${condition}=True"
  else
    fail "${resource} ${condition} did not become True within ${timeout}s"
    return 1
  fi
}

# ---------------------------------------------------------------------------
# assert_jsonpath <resource/name> <jsonpath> <expected> [namespace]
#
# Asserts that a kubectl jsonpath expression returns the expected value.
# Example: assert_jsonpath axonopsplatform/axonops '{.status.timeSeriesSecretName}' 'axonops-timeseries-auth'
# ---------------------------------------------------------------------------
assert_jsonpath() {
  local resource="$1"
  local jsonpath="$2"
  local expected="$3"
  local ns="${4:-${TEST_NAMESPACE}}"

  local actual
  actual=$(kubectl get "${resource}" -n "${ns}" -o "jsonpath=${jsonpath}" 2>/dev/null || true)

  if [[ "${actual}" == "${expected}" ]]; then
    pass "${resource} ${jsonpath} == ${expected}"
  else
    fail "${resource} ${jsonpath}: expected '${expected}', got '${actual}'"
  fi
}

# ---------------------------------------------------------------------------
# assert_jsonpath_nonempty <resource/name> <jsonpath> [namespace]
#
# Asserts that a kubectl jsonpath expression returns a non-empty value.
# ---------------------------------------------------------------------------
assert_jsonpath_nonempty() {
  local resource="$1"
  local jsonpath="$2"
  local ns="${3:-${TEST_NAMESPACE}}"

  local actual
  actual=$(kubectl get "${resource}" -n "${ns}" -o "jsonpath=${jsonpath}" 2>/dev/null || true)

  if [[ -n "${actual}" ]]; then
    pass "${resource} ${jsonpath} is non-empty (${actual})"
  else
    fail "${resource} ${jsonpath} is empty"
  fi
}

# ---------------------------------------------------------------------------
# assert_exists <resource/name> [namespace]
# assert_not_exists <resource/name> [namespace]
# ---------------------------------------------------------------------------
assert_exists() {
  local resource="$1"
  local ns="${2:-${TEST_NAMESPACE}}"

  if kubectl get "${resource}" -n "${ns}" &>/dev/null; then
    pass "${resource} exists"
  else
    fail "${resource} does not exist"
  fi
}

assert_not_exists() {
  local resource="$1"
  local ns="${2:-${TEST_NAMESPACE}}"

  if ! kubectl get "${resource}" -n "${ns}" &>/dev/null; then
    pass "${resource} does not exist (expected)"
  else
    fail "${resource} still exists (expected removal)"
  fi
}

# ---------------------------------------------------------------------------
# apply_and_wait <condition> <resource/name> <timeout_seconds>
#
# Reads YAML from stdin, applies it, then waits for the condition.
# Example:
#   apply_and_wait Ready axonopsmetricalert/e2e-metric-alert 120 <<'EOF'
#   apiVersion: ...
#   EOF
# ---------------------------------------------------------------------------
apply_and_wait() {
  local condition="$1"
  local resource="$2"
  local timeout="${3:-${TIMEOUT}}"

  kubectl apply -f - -n "${TEST_NAMESPACE}"
  wait_for_condition "${resource}" "${condition}" "${timeout}"
}

# ---------------------------------------------------------------------------
# delete_and_wait <resource/name> <timeout_seconds> [namespace]
#
# Deletes a resource and waits for it to be fully removed (finalizer completion).
# ---------------------------------------------------------------------------
delete_and_wait() {
  local resource="$1"
  local timeout="${2:-${TIMEOUT}}"
  local ns="${3:-${TEST_NAMESPACE}}"

  echo "  Deleting ${resource} and waiting for removal ..."
  kubectl delete "${resource}" -n "${ns}" --ignore-not-found
  if kubectl wait "${resource}" \
      --for=delete \
      -n "${ns}" \
      --timeout="${timeout}s" 2>/dev/null; then
    pass "${resource} fully removed"
  else
    fail "${resource} not removed within ${timeout}s"
  fi
}

# ---------------------------------------------------------------------------
# require_env_vars <VAR1> [VAR2 ...]
#
# Fails fast if any listed env var is unset or empty.
# ---------------------------------------------------------------------------
require_env_vars() {
  for var in "$@"; do
    if [[ -z "${!var:-}" ]]; then
      _red "ERROR: required environment variable '${var}' is not set"
      exit 1
    fi
  done
}
