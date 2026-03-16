# AxonOps Operator Examples

This directory contains example deployments demonstrating different use cases for the AxonOps operator.

## Examples

### 1. quickstart.yaml

**Use Case**: Getting started with AxonOps, development/testing environments

**Features**:
- Single replica for all components
- Basic Ingress configuration (no TLS)
- Minimal storage requirements
- 10Gi TimeSeries, 5Gi Search
- Includes sample alerts for metric, log, and TCP health checks

**Setup**:
```bash
kubectl create namespace axonops
kubectl apply -f examples/axonops/quickstart.yaml
```

**Access Dashboard**:
```bash
# Add to /etc/hosts or your DNS:
# 127.0.0.1  axonops.local

# Port-forward if not using Ingress:
kubectl port-forward -n axonops svc/server-dashboard 3000:3000
# Visit http://localhost:3000
```

---

### 2. medium.yaml

**Use Case**: Production deployments with high availability

**Features**:
- Multi-replica setup (HA):
  - Server: 3 replicas
  - TimeSeries: 3 replicas
  - Search: 2 replicas
  - Dashboard: 2 replicas
- TLS certificates via cert-manager and Let's Encrypt
- Proper resource requests/limits
- 100Gi Server, 500Gi TimeSeries, 250Gi Search storage
- API and Agent Ingress with TLS
- Alert routing with PagerDuty and Slack
- Comprehensive metric and log alerts
- TCP and HTTP health checks

**Setup**:
```bash
# 1. Install cert-manager (if not already installed)
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace

# 2. Create namespace and apply manifests
kubectl create namespace axonops-prod
kubectl apply -f examples/axonops/medium.yaml
```

**Access**:
- API: `https://api.axonops.example.com` (replace with your domain)
- Dashboard: `https://dashboard.axonops.example.com`
- Agent: `https://agent.axonops.example.com`

**Prerequisites**:
- DNS configured for your domains
- cert-manager installed
- Nginx Ingress Controller (or similar)

---

### 3. complex.yaml

**Use Case**: Enterprise deployments with advanced features

**Features**:
- Ultra-HA setup (5x Server, 5x TimeSeries, 3x Search, 3x Dashboard)
- Gateway API (Istio) instead of Ingress
- Advanced TLS with Vault issuer
- Large storage: 500Gi Server, 1Ti TimeSeries, 500Gi Search
- JVM tuning (16GB heap for TimeSeries)
- Multi-cluster support (Cassandra US-East, Kafka)
- Sophisticated alert routing:
  - PagerDuty, Slack, OpsGenie integration
  - Different routing for Critical/Warning/Info
  - Escalation paths for on-call
- Comprehensive health checks:
  - TCP checks (native port, internode communication)
  - HTTP checks (JMX endpoints)
  - Shell-based custom checks
- Multi-region filtering (data centers, racks, topics)

**Setup**:
```bash
# 1. Install Istio (if not already installed)
curl -L https://istio.io/downloadIstio | sh -
cd istio-*
./bin/istioctl install --set profile=production -y

# 2. Install cert-manager with Vault support
helm repo add jetstack https://charts.jetstack.io
helm install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace

# 3. Configure Vault issuer (see notes below)

# 4. Create namespace and apply manifests
kubectl create namespace axonops-enterprise
kubectl apply -f examples/axonops/complex.yaml
```

**Vault Configuration** (if using Vault for TLS):
```bash
# Enable PKI secret engine
vault secrets enable pki
vault write pki/roles/kubernetes \
  allowed_domains=axonops.example.com \
  allow_subdomains=true \
  max_ttl=720h
```

**Access**:
- API: `https://api.axonops.example.com` (via Istio Gateway)
- Dashboard: `https://dashboard.axonops.example.com` (via Istio Gateway)
- Agent: `agent.axonops.example.com:1888` (via Istio Gateway)

**Prerequisites**:
- Istio installed and configured
- cert-manager with Vault issuer
- DNS configuration
- API credentials with appropriate integrations configured in AxonOps
- Large cluster resources (recommend 16+ cores, 64GB RAM minimum)

---

## Configuration Reference

### Server Component
```yaml
spec:
  server:
    orgName: "Organization Name"        # Required
    replicas: 3                         # Number of server instances
    repository:
      image: "..."                      # Docker image
      tag: "..."                        # Image tag
      pullPolicy: "IfNotPresent"        # Pull policy
    resources:
      requests:
        cpu: "2"
        memory: "4Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
    storage:
      size: "100Gi"
      storageClassName: "fast-ssd"
    apiIngress:                         # Optional
      enabled: true
      hosts: ["api.example.com"]
      tls: [...]                        # TLS configuration
    agentIngress:                       # Optional
      enabled: true
      hosts: ["agent.example.com"]
    apiGateway:                         # Optional (Gateway API)
      enabled: true
      gatewayClassName: "istio"
      hostname: "api.example.com"
    agentGateway:                       # Optional (Gateway API)
      enabled: true
      gatewayClassName: "istio"
      hostname: "agent.example.com"
```

### Dashboard Component
```yaml
spec:
  dashboard:
    replicas: 2
    repository:
      image: "..."
      tag: "..."
    resources: {...}
    ingress:                            # Optional
      enabled: true
      hosts: ["dashboard.example.com"]
    gateway:                            # Optional (Gateway API)
      enabled: true
      gatewayClassName: "istio"
      hostname: "dashboard.example.com"
```

### TimeSeries & Search
```yaml
spec:
  timeSeries:
    replicas: 3
    storage:
      size: "500Gi"
      storageClassName: "premium-ssd"
    jvmHeapSize: "16g"                  # JVM heap tuning
  search:
    replicas: 2
    storage:
      size: "250Gi"
      storageClassName: "premium-ssd"
```

---

## Scaling Guide

### Small Deployment (Dev/Test)
- See: `quickstart.yaml`
- 1 replica each
- 10-50Gi total storage

### Medium Deployment (Production)
- See: `medium.yaml`
- 2-3 replicas for HA
- 500Gi+ total storage
- TLS certificates

### Large Deployment (Enterprise)
- See: `complex.yaml`
- 5+ replicas for high availability
- 1TB+ total storage
- Gateway API with advanced routing
- Multiple cluster support

---

## Troubleshooting

### Check deployment status
```bash
kubectl get all -n axonops-prod
kubectl describe axonopsserver axonops -n axonops-prod
```

### View logs
```bash
# Server logs
kubectl logs -n axonops-prod statefulset/axonops-server

# Dashboard logs
kubectl logs -n axonops-prod deployment/axonops-dashboard

# Operator logs
kubectl logs -n axonops-system deployment/axonops-controller-manager
```

### Access shell for debugging
```bash
kubectl exec -it -n axonops-prod axonops-server-0 -- bash
```

### Check certificate status
```bash
kubectl get certificate -n axonops-prod
kubectl describe certificate axonops-api-tls -n axonops-prod
```

---

## Common Customizations

### Change storage size
```yaml
spec:
  timeSeries:
    storage:
      size: "1Ti"  # Increase to 1TB
```

### Adjust resource limits
```yaml
spec:
  server:
    resources:
      requests:
        cpu: "4"
        memory: "8Gi"
      limits:
        cpu: "8"
        memory: "16Gi"
```

### Use different storage class
```yaml
spec:
  timeSeries:
    storage:
      storageClassName: "fast-nvme"
```

### Configure custom JVM heap
```yaml
spec:
  timeSeries:
    jvmHeapSize: "32g"  # For large deployments
```

---

## Security Best Practices

1. **Use TLS/HTTPS** - Enable Ingress TLS or Gateway API with certificates
2. **Restrict credentials** - Store API keys in Kubernetes Secrets
3. **RBAC** - Limit operator permissions to required namespaces
4. **Network policies** - Restrict traffic between pods
5. **Storage encryption** - Use encrypted storage classes if available
6. **Regular backups** - Back up persistent volume data

---

## Next Steps

1. Choose an example matching your use case
2. Customize the values (domain names, storage sizes, replicas)
3. Apply the manifest: `kubectl apply -f example.yaml`
4. Verify deployment: `kubectl get axonopsserver`
5. Access the dashboard and configure alerts
