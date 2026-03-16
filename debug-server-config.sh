#!/bin/bash
# Debugging script to diagnose server config secret mounting issue

set -e

NAMESPACE="${1:-axonops-operator-system}"
SERVER_NAME="${2:-server}"

echo "=== Diagnosing AxonOpsServer Config Secret Issue ==="
echo "Namespace: $NAMESPACE"
echo "Server Name: $SERVER_NAME"
echo ""

# Check if AxonOpsServer CR exists
echo "1. Checking AxonOpsServer CR..."
if kubectl get axonopsserver -n "$NAMESPACE" "$SERVER_NAME" &>/dev/null; then
    echo "✓ AxonOpsServer exists"
    kubectl get axonopsserver -n "$NAMESPACE" "$SERVER_NAME" -o yaml | grep -A 20 "spec:"
else
    echo "✗ AxonOpsServer not found"
    exit 1
fi
echo ""

# Check if config secret exists
echo "2. Checking config secret..."
CONFIG_SECRET="${SERVER_NAME}-${SERVER_NAME}"
if kubectl get secret -n "$NAMESPACE" "$CONFIG_SECRET" &>/dev/null; then
    echo "✓ Config secret exists: $CONFIG_SECRET"
    echo "  Keys in secret:"
    kubectl get secret -n "$NAMESPACE" "$CONFIG_SECRET" -o jsonpath='{.data}' | jq 'keys'
else
    echo "✗ Config secret NOT found: $CONFIG_SECRET"
    echo "  Available secrets:"
    kubectl get secrets -n "$NAMESPACE" | grep "$SERVER_NAME"
fi
echo ""

# Check StatefulSet
echo "3. Checking StatefulSet..."
STS_NAME="${SERVER_NAME}-${SERVER_NAME}"
if kubectl get sts -n "$NAMESPACE" "$STS_NAME" &>/dev/null; then
    echo "✓ StatefulSet exists: $STS_NAME"
    echo "  Volume mounts:"
    kubectl get sts -n "$NAMESPACE" "$STS_NAME" -o jsonpath='{.spec.template.spec.containers[0].volumeMounts}' | jq '.'
    echo "  Volumes:"
    kubectl get sts -n "$NAMESPACE" "$STS_NAME" -o jsonpath='{.spec.template.spec.volumes}' | jq '.'
else
    echo "✗ StatefulSet NOT found: $STS_NAME"
fi
echo ""

# Check Pod
echo "4. Checking Pod..."
POD_NAME="${SERVER_NAME}-${SERVER_NAME}-0"
if kubectl get pod -n "$NAMESPACE" "$POD_NAME" &>/dev/null; then
    echo "✓ Pod exists: $POD_NAME"
    echo "  Volume mounts in pod:"
    kubectl get pod -n "$NAMESPACE" "$POD_NAME" -o jsonpath='{.spec.containers[0].volumeMounts}' | jq '.'
    echo "  Volumes in pod:"
    kubectl get pod -n "$NAMESPACE" "$POD_NAME" -o jsonpath='{.spec.volumes}' | jq '.'

    echo "  Checking if /etc/axonops exists in container:"
    if kubectl exec -n "$NAMESPACE" "$POD_NAME" -- test -d /etc/axonops; then
        echo "  ✓ /etc/axonops directory exists"
        echo "    Contents:"
        kubectl exec -n "$NAMESPACE" "$POD_NAME" -- ls -la /etc/axonops || echo "    (cannot read)"
    else
        echo "  ✗ /etc/axonops directory NOT found"
    fi
else
    echo "✗ Pod NOT found: $POD_NAME"
    echo "  Available pods:"
    kubectl get pods -n "$NAMESPACE" | grep "$SERVER_NAME"
fi
echo ""

# Check operator logs
echo "5. Checking operator logs for reconciliation errors..."
OPERATOR_POD=$(kubectl get pods -n "$NAMESPACE" -l control-plane=controller-manager -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
if [ -n "$OPERATOR_POD" ]; then
    echo "✓ Operator pod: $OPERATOR_POD"
    echo "  Recent errors (last 50 lines):"
    kubectl logs -n "$NAMESPACE" "$OPERATOR_POD" --tail=50 | grep -i "error\|failed" || echo "    (no errors found)"
else
    echo "✗ Operator pod not found"
fi
