Feature: AxonOpsMetricAlert CRUD lifecycle with AxonOps API synchronization
  As an operator user
  I want to define metric threshold alerts in Kubernetes using AxonOpsMetricAlert CRs
  And have the operator automatically sync them to the AxonOps API
  So that Cassandra/OpenSearch metrics are monitored according to Kubernetes-native configuration

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create AxonOpsMetricAlert
    Given an AxonOpsMetricAlert CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.clusterType set to "cassandra"
      - spec.metric set to "cpu_usage"
      - spec.warningValue set to 75
      - spec.criticalValue set to 90
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOps API should be called to create an alert rule
    And the AxonOpsMetricAlert status should have field "syncedAlertID" with the returned alert ID
    And the status condition should show "Ready=True"

  Scenario: Update metric alert warningValue
    Given an existing AxonOpsMetricAlert CR with syncedAlertID stored in status
    When the user updates the CR to change spec.warningValue from 75 to 80
    And the operator reconciles the AxonOpsMetricAlert CR
    Then the AxonOps API should be called with HTTP PUT to update the alert rule
    And the syncedAlertID should remain unchanged
    And the status condition should show "Ready=True"

  Scenario: Delete metric alert
    Given an existing AxonOpsMetricAlert CR with syncedAlertID in status
    When the user deletes the AxonOpsMetricAlert CR
    Then the operator should call the AxonOps API to delete the alert rule
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Missing AxonOpsConnection reference
    Given an AxonOpsMetricAlert CR with spec.connectionRef set to "nonexistent"
    And no AxonOpsConnection with that name exists
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "ConnectionNotFound"
    And the alert should not be synced to the AxonOps API

  Scenario: AxonOps API returns error on create
    Given an AxonOpsMetricAlert CR configured correctly
    And the AxonOps API returns a 400 Bad Request error
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "SyncFailed"
    And the condition message should include the API error details
    And the operator should requeue for retry

  Scenario: Required clusterType field prevents reconciliation
    Given an AxonOpsMetricAlert CR with spec.clusterType omitted
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the status condition should show "Ready=False"
    And the condition reason should indicate "MissingClusterType" or similar
    And the alert should not be synced

  Scenario: Dashboard/chart/metric resolved to panel UUID
    Given an AxonOpsMetricAlert CR referencing a specific dashboard and metric
    And the AxonOps API supports template resolution
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the operator should call the AxonOps template API to resolve the metric to a panel UUID
    And the resolved UUID should be stored in the alert configuration
    And the synced alert in AxonOps should reference the correct panel
