Feature: Ingress and Gateway API exposure for AxonOpsServer components
  As a platform operator
  I want to configure external access to AxonOpsServer components (Dashboard, Server API, Server Agent)
  Using either traditional Ingress or modern Gateway API resources
  So that users can access the observability platform securely and with flexible routing

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Enable dashboard Ingress
    Given an AxonOpsServer CR with spec.dashboard.ingress.enabled set to true
    And spec.dashboard.ingress.host set to "dashboard.example.com"
    And spec.dashboard.ingress.path set to "/"
    When the operator reconciles the AxonOpsServer CR
    Then an Ingress resource should be created for the Dashboard
    And the Ingress should route "dashboard.example.com/" to the axon-dash Service
    And the Ingress should be owned by the AxonOpsServer CR

  Scenario: Enable server agent port Ingress
    Given an AxonOpsServer CR with spec.server.agentIngress.enabled set to true
    And spec.server.agentIngress.host set to "agent.example.com"
    When the operator reconciles the AxonOpsServer CR
    Then an Ingress resource should be created for the Server agent
    And the Ingress should route to port 1888 of the axon-server Service
    And the AxonOpsServer status should reflect "AgentIngressReady=True"

  Scenario: Enable server API port Ingress
    Given an AxonOpsServer CR with spec.server.apiIngress.enabled set to true
    And spec.server.apiIngress.host set to "api.example.com"
    When the operator reconciles the AxonOpsServer CR
    Then an Ingress resource should be created for the Server API
    And the Ingress should route to port 8080 of the axon-server Service
    And the Ingress path should allow API calls to reach the Server

  Scenario: Disable previously enabled dashboard Ingress
    Given an existing AxonOpsServer CR with Dashboard Ingress enabled
    And the Dashboard Ingress resource is created
    When the user updates the CR to set spec.dashboard.ingress.enabled to false
    And the operator reconciles the AxonOpsServer CR
    Then the Dashboard Ingress should be deleted
    And the axon-dash Service should still exist
    And the AxonOpsServer status should reflect the Ingress removal

  Scenario: Enable Gateway API for dashboard
    Given an AxonOpsServer CR with spec.dashboard.gateway.enabled set to true
    And spec.dashboard.gateway.gatewayClassName set to "nginx" (or any valid class)
    And spec.dashboard.gateway.hosts set to ["dashboard.example.com"]
    When the operator reconciles the AxonOpsServer CR
    Then a Gateway resource should be created (or referenced if existing)
    And an HTTPRoute resource should be created for the Dashboard
    And the HTTPRoute should route "dashboard.example.com" to the axon-dash Service on port 3000
    And both resources should be owned by the AxonOpsServer CR

  Scenario: Enable Gateway API for server agent
    Given an AxonOpsServer CR with spec.server.agentGateway.enabled set to true
    And spec.server.agentGateway.gatewayClassName and hosts configured
    When the operator reconciles the AxonOpsServer CR
    Then an HTTPRoute should be created for the Server agent
    And the HTTPRoute should route traffic to port 1888 of the axon-server Service
    And the AxonOpsServer status should reflect "AgentGatewayReady=True"

  Scenario: Enable Gateway API for server API
    Given an AxonOpsServer CR with spec.server.apiGateway.enabled set to true
    And spec.server.apiGateway.gatewayClassName and hosts configured
    When the operator reconciles the AxonOpsServer CR
    Then an HTTPRoute should be created for the Server API
    And the HTTPRoute should route traffic to port 8080 of the axon-server Service
    And the AxonOpsServer status should reflect "ApiGatewayReady=True"

  Scenario: Both Ingress and Gateway can be enabled simultaneously
    Given an AxonOpsServer CR with:
      - spec.dashboard.ingress.enabled set to true
      - spec.dashboard.gateway.enabled set to true
    When the operator reconciles the AxonOpsServer CR
    Then both the Ingress and HTTPRoute resources should be created
    And they should be independent and not interfere with each other
    And the AxonOpsServer status should show both as "Ready=True"

  Scenario: Missing gatewayClassName when Gateway enabled
    Given an AxonOpsServer CR with spec.dashboard.gateway.enabled set to true
    And spec.dashboard.gateway.gatewayClassName is omitted or empty
    When the operator reconciles the AxonOpsServer CR
    Then the AxonOpsServer status should have condition "DashboardGatewayReady=False"
    And the condition reason should be "MissingGatewayClassName"
    And the condition message should explain the required field
    And reconciliation should not proceed for the Gateway/HTTPRoute
