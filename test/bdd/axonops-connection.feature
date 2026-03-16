Feature: AxonOpsConnection lifecycle for alert controller credential reuse
  As an operator user
  I want to define a centralized AxonOpsConnection resource that holds AxonOps API credentials
  And reference it from multiple alert CRs rather than duplicating credentials
  So that I can manage API access through a single point and rotate credentials easily

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Create AxonOpsConnection with valid apiKeyRef
    Given a Secret named "axonops-api" with key "apiKey" containing a valid API token
    And an AxonOpsConnection CR with:
      - spec.apiKeyRef set to "axonops-api"
      - spec.host set to "api.axonops.com"
      - spec.protocol set to "https"
    When the operator reconciles the AxonOpsConnection CR
    Then the AxonOpsConnection status should have condition "Ready=True"
    And the CR should be usable by alert controllers

  Scenario: Referenced Secret does not exist
    Given an AxonOpsConnection CR with spec.apiKeyRef set to "nonexistent-secret"
    And no Secret named "nonexistent-secret" exists
    When the operator reconciles the AxonOpsConnection CR
    Then the AxonOpsConnection status should have condition "Ready=False"
    And the condition reason should be "SecretNotFound"
    And the condition message should include the missing Secret name

  Scenario: Update apiKeyRef to a new Secret
    Given an existing AxonOpsConnection CR referencing Secret "old-api-key"
    And the CR status shows "Ready=True"
    When the user updates the CR to reference "new-api-key" Secret
    And a Secret named "new-api-key" exists with valid API credentials
    And the operator reconciles the AxonOpsConnection CR
    Then the AxonOpsConnection status should update to "Ready=True"
    And the new API credentials should be used for subsequent API calls
    And alert controllers using this connection should pick up the new credentials

  Scenario: Default host to dash.axonops.cloud when omitted
    Given an AxonOpsConnection CR with:
      - spec.apiKeyRef set to a valid Secret
      - spec.host is omitted
    When the operator reconciles the AxonOpsConnection CR
    Then the AxonOpsConnection status should show effective host as "dash.axonops.cloud"
    And the CR should be functional with the default host

  Scenario: Default protocol to https when omitted
    Given an AxonOpsConnection CR with:
      - spec.apiKeyRef set to a valid Secret
      - spec.protocol is omitted
    When the operator reconciles the AxonOpsConnection CR
    Then the AxonOpsConnection status should reflect protocol as "https"
    And API calls should use HTTPS connections

  Scenario: Delete AxonOpsConnection while alert CRs reference it
    Given an existing AxonOpsConnection CR used by multiple alert CRs
    And at least one AxonOpsMetricAlert references this connection
    When the user deletes the AxonOpsConnection CR
    Then the alert CRs should detect the missing connection
    And their status conditions should show "ConnectionNotFound"
    And the alerts should stop syncing to the AxonOps API
    And the deletion should succeed (no finalizer blocks it)

  Scenario: Multiple alert CRs can reference the same AxonOpsConnection
    Given an AxonOpsConnection CR with valid credentials
    And three different alert CRs all referencing this connection by name
    When the operator reconciles the AxonOpsConnection and all alert CRs
    Then all three alert CRs should use the same API credentials
    And API calls should be routed through the AxonOpsConnection reference
    And updating the connection credentials should affect all referencing alerts
