Feature: AxonOpsAlertRoute lifecycle for alert notification routing
  As an operator user
  I want to define alert routes that direct notifications to integration endpoints (Slack, PagerDuty, etc.)
  Using AxonOpsAlertRoute CRs that sync to the AxonOps API
  So that alerts are routed to the correct notification channels based on severity and type

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create AlertRoute with Slack integration
    Given an AxonOpsAlertRoute CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.integrationType set to "slack"
      - spec.integrationName set to "team-alerts"
      - spec.severity set to ["warning", "critical"]
    When the operator reconciles the AxonOpsAlertRoute CR
    Then the AxonOps API should be called to look up the Slack integration by name
    And the integration ID should be stored in status field "integrationID"
    And the AxonOpsAlertRoute status condition should show "Ready=True"

  Scenario: Update alert route severity filter
    Given an existing AxonOpsAlertRoute CR with integrationID in status
    When the user updates the CR to change spec.severity from ["warning"] to ["critical"]
    And the operator reconciles the AxonOpsAlertRoute CR
    Then the AxonOps API should be called with HTTP PUT to update the route
    And the severity filter should be updated to only "critical" alerts
    And the integrationID should remain unchanged

  Scenario: Delete alert route
    Given an existing AxonOpsAlertRoute CR with integrationID in status
    When the user deletes the AxonOpsAlertRoute CR
    Then the operator should call the AxonOps API to delete the route
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Unsupported integrationType value
    Given an AxonOpsAlertRoute CR with spec.integrationType set to "unsupported_service"
    When the operator reconciles the AxonOpsAlertRoute CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "UnsupportedIntegrationType" or similar
    And the route should not be synced to the AxonOps API

  Scenario: Integration not found by name
    Given an AxonOpsAlertRoute CR with:
      - spec.integrationType set to "slack"
      - spec.integrationName set to "nonexistent-channel"
    And the AxonOps API returns "not found" for this integration
    When the operator reconciles the AxonOpsAlertRoute CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "IntegrationNotFound"
    And the condition message should indicate the missing integration name

  Scenario: Global route type routes all alert types
    Given an AxonOpsAlertRoute CR with:
      - spec.routeType set to "global"
      - spec.integrationType and integrationName configured
      - spec.severity omitted (default to all)
    When the operator reconciles the AxonOpsAlertRoute CR
    Then the route should be configured to receive all alert severity levels
    And the route should apply to both metric and log alerts
    And the AxonOpsAlertRoute status should show "Ready=True"

  Scenario: Multiple routes can target the same integration
    Given a Slack integration that exists in AxonOps
    And two different AxonOpsAlertRoute CRs both configured for this Slack channel
    And one routes "critical" alerts and the other routes "warning" alerts
    When the operator reconciles both routes
    Then both routes should be synced to the AxonOps API
    And they should have independent route IDs
    And the Slack channel should receive alerts according to both route configurations
