Feature: Alert controller error handling and retry behavior
  As a Kubernetes operator
  I need to handle AxonOps API errors gracefully with appropriate retry behavior
  So that transient failures are retried and permanent failures are reported clearly

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And an AxonOpsConnection "test-connection" exists with valid credentials

  Scenario: Retryable API error triggers requeue after 30 seconds
    Given an AxonOpsMetricAlert CR referencing "test-connection"
    And the AxonOps API returns a retryable error (500 Internal Server Error)
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOpsMetricAlert status condition "Failed" should be set
    And the reconciliation should be requeued after 30 seconds

  Scenario: Non-retryable API error on create sets Failed condition
    Given an AxonOpsMetricAlert CR referencing "test-connection"
    And the AxonOps API returns a non-retryable error (400 Bad Request)
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOpsMetricAlert status condition "Failed" should be set
    And the status message should describe the API error

  Scenario: Non-retryable error on delete allows finalizer removal
    Given an AxonOpsMetricAlert CR with SyncedAlertID "alert-123"
    And the AxonOps API returns 404 Not Found when deleting alert "alert-123"
    When I delete the AxonOpsMetricAlert CR
    Then the finalizer should be removed despite the API error
    And the AxonOpsMetricAlert CR should be fully deleted

  Scenario: Skip API call when generation unchanged and already Ready
    Given an AxonOpsMetricAlert CR that was previously synced successfully
    And the CR generation has not changed since last sync
    And the status condition "Ready" is true
    When the operator reconciles the AxonOpsMetricAlert CR
    Then no API call should be made to AxonOps
    And the status should remain unchanged

  Scenario: Connection not found sets error condition
    Given an AxonOpsMetricAlert CR referencing connection "nonexistent-connection"
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOpsMetricAlert status condition "Failed" should be set
    And the status message should indicate the connection was not found

  Scenario: API key Secret not found sets error condition
    Given an AxonOpsConnection "broken-connection" referencing Secret "missing-secret"
    And an AxonOpsMetricAlert CR referencing "broken-connection"
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOpsMetricAlert status condition "Failed" should be set
    And the status message should indicate the API key Secret was not found

  Scenario: Dashboard panel resolution failure sets error condition
    Given an AxonOpsMetricAlert CR with dashboard "nonexistent-dashboard"
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOpsMetricAlert status condition "Failed" should be set
    And the status message should indicate the dashboard panel could not be resolved
