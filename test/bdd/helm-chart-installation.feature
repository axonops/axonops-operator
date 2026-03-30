Feature: Helm chart for AxonOps operator installation
  As a Kubernetes cluster administrator
  I want to install the AxonOps operator using Helm
  So that I can manage operator deployment declaratively and integrate it with my Helm-based infrastructure

  Background:
    Given a Kubernetes cluster with Helm 3.x installed
    And the AxonOps operator Helm chart is available in a repository

  Scenario: Install operator with default configuration
    Given a Helm chart for the AxonOps operator
    And default values.yaml configured for standard deployment
    When the user runs "helm install axonops-operator <chart-path>"
    Then the operator manager Deployment should be created in "axonops-operator-system" namespace
    And the ServiceAccount "axonops-operator-controller-manager" should be created
    And the ClusterRole and ClusterRoleBinding for RBAC should be created
    And the operator should be running with 1 replica by default
    And the CRDs should be installed automatically

  Scenario: Install operator with custom image and tag
    Given a Helm chart with configurable image values
    When the user runs "helm install axonops-operator <chart-path> --set image.repository=myregistry.com/axonops-operator --set image.tag=v1.2.3"
    Then the operator manager Deployment should use the custom image
    And the container spec should reference "myregistry.com/axonops-operator:v1.2.3"

  Scenario: Configure resource requests and limits
    Given a Helm chart with customizable resource specifications
    When the user runs "helm install axonops-operator <chart-path> --set resources.requests.cpu=250m --set resources.requests.memory=256Mi --set resources.limits.cpu=500m --set resources.limits.memory=512Mi"
    Then the operator manager Deployment should specify the custom resource requests
    And the operator manager Deployment should specify the custom resource limits

  Scenario: Configure manager replica count for high availability
    Given a Helm chart supporting multiple replicas
    When the user runs "helm install axonops-operator <chart-path> --set replicas=3"
    Then the operator manager Deployment should have 3 replicas
    And a PodDisruptionBudget should be created to maintain quorum during disruptions

  Scenario: Configure log level
    Given a Helm chart with configurable log level
    When the user runs "helm install axonops-operator <chart-path> --set logLevel=debug"
    Then the operator manager Pod should be started with debug-level logging
    And verbose output should appear in pod logs

  Scenario: Upgrade operator to new version
    Given an existing AxonOps operator installed via Helm at version v1.0.0
    And a new version v1.1.0 is available in the Helm repository
    When the user runs "helm upgrade axonops-operator <chart-path> --set image.tag=v1.1.0"
    Then the operator manager Deployment should be updated with the new image
    And the CRDs should be updated to the v1.1.0 schema
    And existing AxonOpsPlatform and alert CRs should remain functional

  Scenario: Rollback operator to previous version
    Given an AxonOps operator upgraded to a problematic version
    When the user runs "helm rollback axonops-operator 1"
    Then the operator manager Deployment should revert to the previous version
    And the CRDs should revert to the previous schema version
    And all existing CRs should remain intact

  Scenario: Uninstall operator via Helm
    Given an existing AxonOps operator installed via Helm
    When the user runs "helm uninstall axonops-operator"
    Then the operator manager Deployment should be deleted
    And the ServiceAccount, Roles, and RoleBindings should be deleted
    And the CRDs should remain in the cluster (with existing CRs preserved)

  Scenario: Helm chart passes linting and validation
    Given a Helm chart for the AxonOps operator
    When the user runs "helm lint <chart-path>"
    Then the chart should pass all linting checks
    And there should be no warnings or errors
    When the user runs "helm template axonops-operator <chart-path>"
    Then the generated Kubernetes manifests should be valid YAML

  Scenario: Chart includes all required RBAC permissions
    Given a Helm chart for the AxonOps operator
    When the user runs "helm template axonops-operator <chart-path>"
    Then a ClusterRole should be generated with permissions for:
      - core.axonops.com resources (get, list, watch, create, update, patch, delete)
      - alerts.axonops.com resources (get, list, watch, create, update, patch, delete)
      - Secrets (get, list, watch, create, update, patch)
      - StatefulSets, Deployments, Services, Ingress (full CRUD)
      - Certificates (cert-manager integration)
    And a ClusterRoleBinding should bind this role to the ServiceAccount

  Scenario: Configure webhook certificate management
    Given a Helm chart with webhook support
    And cert-manager is installed in the cluster
    When the user runs "helm install axonops-operator <chart-path> --set webhooks.enabled=true"
    Then the webhook Service and Secret should be created
    And cert-manager should generate a self-signed Certificate for the webhook
    And the ValidatingWebhookConfiguration and MutatingWebhookConfiguration should reference the correct service

  Scenario: Install with custom namespace
    Given a Helm chart supporting namespace configuration
    When the user runs "helm install axonops-operator <chart-path> --namespace custom-ns --create-namespace"
    Then the operator should be deployed in the "custom-ns" namespace
    And the ServiceAccount and Roles should be created in "custom-ns"
    And the ClusterRole and ClusterRoleBinding should reference the "custom-ns" ServiceAccount

  Scenario: Helm values file includes documentation
    Given a default values.yaml for the AxonOps operator Helm chart
    When the user reviews the values.yaml file
    Then each configurable value should have a comment explaining its purpose
    And default values should be production-ready
    And examples should be provided for common customizations
