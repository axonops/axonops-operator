Feature: AxonOpsAdaptiveRepair lifecycle for Cassandra/DSE adaptive repair settings
  As an operator user
  I want to declare adaptive repair settings using AxonOpsAdaptiveRepair CRs
  So that repair configuration is managed declaratively via GitOps

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR "my-connection" exists in namespace "default"

  Scenario: Create adaptive repair configuration with default settings
    Given an AxonOpsAdaptiveRepair CR with:
      | field          | value         |
      | connectionRef  | my-connection |
      | clusterName    | my-cluster    |
      | clusterType    | cassandra     |
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the AxonOps API should receive a POST to /api/v1/adaptiveRepair
    And the API payload should contain:
      | field              | value |
      | Active             | true  |
      | GcGraceThreshold   | 86400 |
      | TableParallelism   | 10    |
      | FilterTWCSTables   | true  |
      | SegmentRetries     | 3     |
      | SegmentTargetSizeMB| 0     |
      | SegmentTimeout     | 2h    |
      | MaxSegmentsPerTable| 0     |
    And the status condition "Ready" should be "True"
    And the status should contain a lastSyncTime
    And the status observedGeneration should match the CR generation

  Scenario: Create adaptive repair with custom settings
    Given an AxonOpsAdaptiveRepair CR with:
      | field              | value            |
      | connectionRef      | my-connection    |
      | clusterName        | my-cluster       |
      | clusterType        | dse              |
      | active             | true             |
      | tableParallelism   | 2                |
      | gcGraceThreshold   | 172800           |
      | segmentTargetSizeMB| 64               |
      | excludedTables     | ["ks.tbl1"]      |
      | filterTWCSTables   | false            |
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the AxonOps API should receive a POST to /api/v1/adaptiveRepair
    And the API payload should contain:
      | field              | value       |
      | Active             | true        |
      | TableParallelism   | 2           |
      | GcGraceThreshold   | 172800      |
      | SegmentTargetSizeMB| 64          |
      | BlacklistedTables  | ["ks.tbl1"] |
      | FilterTWCSTables   | false       |
    And the status condition "Ready" should be "True"

  Scenario: Disable adaptive repair
    Given an existing AxonOpsAdaptiveRepair CR with Ready=True
    When the user updates the CR to set active=false
    And the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the AxonOps API should receive a POST containing Active=false
    And the status condition "Ready" should be "True"

  Scenario: Update excluded tables
    Given an existing AxonOpsAdaptiveRepair CR with Ready=True and excludedTables=[]
    When the user updates the CR to set excludedTables=["system.local","system.peers"]
    And the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the AxonOps API should receive a POST containing BlacklistedTables=["system.local","system.peers"]
    And the status condition "Ready" should be "True"

  Scenario: No API call when spec has not changed
    Given an existing AxonOpsAdaptiveRepair CR with Ready=True
    And the status observedGeneration matches the CR generation
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then no API call should be made to the AxonOps adaptive repair endpoint
    And the status should remain unchanged

  Scenario: Drift correction when remote settings differ from spec
    Given an existing AxonOpsAdaptiveRepair CR with tableParallelism=10
    And the remote AxonOps API returns TableParallelism=5
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the AxonOps API should receive a POST containing TableParallelism=10
    And the status condition "Ready" should be "True"

  Scenario: No drift when remote matches spec
    Given an existing AxonOpsAdaptiveRepair CR with default settings
    And the remote AxonOps API returns matching default settings
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then no POST should be sent to the AxonOps adaptive repair endpoint
    And the status condition "Ready" should be "True"

  Scenario: Delete adaptive repair CR removes finalizer without API cleanup
    Given an existing AxonOpsAdaptiveRepair CR with Ready=True
    When the user deletes the AxonOpsAdaptiveRepair CR
    And the operator reconciles the deletion
    Then no DELETE or POST request should be sent to the AxonOps API
    And the finalizer "alerts.axonops.com/adaptive-repair-finalizer" should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Failed condition when AxonOpsConnection reference is invalid
    Given an AxonOpsAdaptiveRepair CR with connectionRef="nonexistent"
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the status condition "Failed" should be "True" with reason "ConnectionError"
    And the CR should be requeued for retry after 30 seconds

  Scenario: Failed condition when AxonOps API returns server error on GET
    Given an AxonOpsAdaptiveRepair CR with a valid connection
    And the AxonOps API GET /api/v1/adaptiveRepair returns HTTP 500
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should be requeued for retry after 30 seconds

  Scenario: Failed condition when AxonOps API returns server error on POST
    Given an AxonOpsAdaptiveRepair CR with a valid connection
    And the AxonOps API GET returns valid settings different from spec
    And the AxonOps API POST /api/v1/adaptiveRepair returns HTTP 500
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should be requeued for retry after 30 seconds

  Scenario: Non-retryable API error does not requeue
    Given an AxonOpsAdaptiveRepair CR with a valid connection
    And the AxonOps API POST /api/v1/adaptiveRepair returns HTTP 400
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should NOT be requeued

  Scenario: Invalid cluster type rejected by API server
    Given an AxonOpsAdaptiveRepair CR with clusterType="kafka"
    When the CR is submitted to the Kubernetes API
    Then the API server should reject the CR with a validation error
    And the error should indicate that "kafka" is not a valid value for clusterType

  Scenario: ExcludedTables nil treated as empty for comparison
    Given an AxonOpsAdaptiveRepair CR with excludedTables not set
    And the remote AxonOps API returns BlacklistedTables as null
    When the operator reconciles the AxonOpsAdaptiveRepair CR
    Then no POST should be sent to the AxonOps adaptive repair endpoint
    And the status condition "Ready" should be "True"
