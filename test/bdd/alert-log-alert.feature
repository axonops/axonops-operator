Feature: AxonOpsLogAlert CRUD lifecycle with event pattern matching
  As an operator user
  I want to define log-based alerts that trigger on specific log patterns
  Using AxonOpsLogAlert CRs that sync to the AxonOps API
  So that I can alert on application errors, exceptions, and specific log messages

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create AxonOpsLogAlert with content and level
    Given an AxonOpsLogAlert CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.content set to "OutOfMemoryError"
      - spec.level set to "ERROR"
      - spec.logType set to "system_log"
    When the operator reconciles the AxonOpsLogAlert CR
    Then the AxonOps API should be called to create a log alert rule
    And the alert should use "events{...}" syntax with the pattern in the API call
    And the AxonOpsLogAlert status should have field "syncedAlertID"
    And the status condition should show "Ready=True"

  Scenario: Update log alert content phrase
    Given an existing AxonOpsLogAlert CR with syncedAlertID in status
    When the user updates the CR to change spec.content from "OutOfMemoryError" to "NullPointerException"
    And the operator reconciles the AxonOpsLogAlert CR
    Then the AxonOps API should be called with HTTP PUT to update the alert
    And the updated events filter should include the new content phrase
    And the syncedAlertID should remain unchanged

  Scenario: Delete log alert
    Given an existing AxonOpsLogAlert CR with syncedAlertID in status
    When the user deletes the AxonOpsLogAlert CR
    Then the operator should call the AxonOps API to delete the alert rule
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Missing source field validation
    Given an AxonOpsLogAlert CR without spec.source field
    When the operator reconciles the AxonOpsLogAlert CR
    Then the status condition should show "Ready=False"
    And the condition reason should indicate "MissingSource" or similar
    And the alert should not be synced to the AxonOps API

  Scenario: Multiple log alerts for the same connection
    Given a valid AxonOpsConnection CR
    And two different AxonOpsLogAlert CRs both referencing the same connection
    And one alert triggers on "error" and the other on "warning"
    When the operator reconciles both alerts
    Then both alerts should be synced to the AxonOps API
    And they should use independent alert rule IDs
    And both should share the same AxonOpsConnection credentials

  Scenario: Log alert with special characters in content
    Given an AxonOpsLogAlert CR with spec.content containing regex special characters or quotes
    When the operator reconciles the AxonOpsLogAlert CR
    Then the content should be properly escaped in the API request
    And the alert should be created successfully
    And the AxonOps API should receive the properly formatted events filter
