Feature: Pause reconciliation
  As a cluster administrator
  I want to pause reconciliation of AxonOps resources
  So that I can perform maintenance, debug issues, or prevent the operator from
  overwriting manual changes without deleting or modifying the resources

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the operator is reconciling resources normally

  # --- AxonOpsPlatform pause ---

  Scenario: AxonOpsPlatform reconciliation
    Given an AxonOpsPlatform resource with all components running
    When I set spec.pause to true on the AxonOpsPlatform
    Then the controller should stop reconciling all server components
    And existing StatefulSets, Deployments, Services, and Secrets should remain unchanged
    And the status should have a condition "Paused" with status "True" and reason "ReconciliationPaused"
    And the controller should log that reconciliation is paused

  Scenario: AxonOpsPlatform reconciliation
    Given an AxonOpsPlatform resource with spec.pause set to true
    When I set spec.pause to false
    Then the controller should resume reconciling all server components
    And the "Paused" condition should be removed from status
    And any pending spec changes made while paused should be applied

  Scenario: AxonOpsPlatform ignores spec changes
    Given an AxonOpsPlatform resource with spec.pause set to true
    When I change the server image tag in the spec
    Then the controller should not update the StatefulSet
    And the observedGeneration should not advance
    When I set spec.pause to false
    Then the controller should apply the image tag change

  Scenario: AxonOpsPlatform pause is false by default
    Given a new AxonOpsPlatform resource with no pause field set
    Then spec.pause should default to false
    And the controller should reconcile normally

  Scenario: AxonOpsPlatform still handles deletion
    Given an AxonOpsPlatform resource with spec.pause set to true
    When I delete the AxonOpsPlatform resource
    Then the finalizer should still run
    And all owned resources should be cleaned up normally

  # --- AxonOpsConnection pause ---

  Scenario: Pause AxonOpsConnection reconciliation
    Given an AxonOpsConnection resource referenced by several alert CRDs
    When I set spec.pause to true on the AxonOpsConnection
    Then the connection status should have a condition "Paused" with status "True"

  Scenario: Paused connection blocks dependant resource reconciliation
    Given an AxonOpsConnection with spec.pause set to true
    And an AxonOpsMetricAlert referencing that connection
    When the AxonOpsMetricAlert is reconciled
    Then the controller should skip the API call
    And set condition "Paused" with status "True" and reason "ConnectionPaused" on the alert
    And the alert should retain its existing status (syncedAlertID etc.)
    And the controller should not requeue with error

  Scenario: Paused connection blocks all dependant CRD types
    Given an AxonOpsConnection with spec.pause set to true
    Then all of the following CRDs referencing that connection should be paused:
      | AxonOpsMetricAlert     |
      | AxonOpsLogAlert        |
      | AxonOpsAlertRoute      |
      | AxonOpsAlertEndpoint   |
      | AxonOpsHealthcheckHTTP |
      | AxonOpsHealthcheckTCP  |
      | AxonOpsHealthcheckShell|
      | AxonOpsDashboardTemplate |
      | AxonOpsAdaptiveRepair  |
      | AxonOpsScheduledRepair |
      | AxonOpsCommitlogArchive|
      | AxonOpsSilenceWindow   |
      | AxonOpsBackup          |
      | AxonOpsKafkaTopic      |
      | AxonOpsKafkaACL        |
      | AxonOpsKafkaConnector  |
    And each should have a "Paused" condition with reason "ConnectionPaused"

  Scenario: Resume connection resumes dependant resources
    Given an AxonOpsConnection with spec.pause set to true
    And several alert CRDs referencing that connection are paused
    When I set spec.pause to false on the connection
    Then the "Paused" condition should be removed from the connection
    And the next reconciliation of each alert CRD should proceed normally
    And any spec changes made while paused should be synced to the AxonOps API

  Scenario: AxonOpsConnection pause is false by default
    Given a new AxonOpsConnection resource with no pause field set
    Then spec.pause should default to false
    And dependant resources should reconcile normally

  Scenario: Paused connection still allows deletion of dependant resources
    Given an AxonOpsConnection with spec.pause set to true
    And an AxonOpsMetricAlert referencing that connection
    When I delete the AxonOpsMetricAlert
    Then the finalizer should still call the AxonOps API to delete the alert
    And the resource should be fully removed

  # --- Status print column ---

  Scenario: kubectl shows paused status
    Given an AxonOpsPlatform with spec.pause set to true
    When I run kubectl get axonopsplatforms
    Then the Paused column should show "True"

  Scenario: kubectl shows paused connection
    Given an AxonOpsConnection with spec.pause set to true
    When I run kubectl get axonopsconnections
    Then the Paused column should show "True"

  # --- Edge cases ---

  Scenario: Rapid pause/unpause does not lose events
    Given an AxonOpsPlatform reconciling normally
    When I set spec.pause to true and immediately back to false
    Then the controller should process the final state (unpaused)
    And reconciliation should continue without errors

  Scenario: Pause does not affect other AxonOpsConnections
    Given two AxonOpsConnection resources "conn-a" and "conn-b"
    When I pause "conn-a"
    Then only resources referencing "conn-a" should be paused
    And resources referencing "conn-b" should continue reconciling normally
