Feature: AxonOpsScheduledRepair management
  As a Cassandra cluster administrator
  I need to declaratively manage scheduled repair configurations via Kubernetes CRDs
  So that repair schedules are version-controlled and reconciled automatically

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsConnection "test-connection" exists with valid API credentials
    And a Cassandra cluster "production" is registered in AxonOps

  # --- Basic lifecycle ---

  Scenario: Create a basic scheduled repair
    Given an AxonOpsScheduledRepair CR:
      | field              | value               |
      | connectionRef      | test-connection     |
      | clusterName        | production          |
      | clusterType        | cassandra           |
      | tag                | monthly-full        |
      | scheduleExpression | 0 0 1 * *           |
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the AxonOps API should receive a POST to "/api/v1/addrepair/{org}/cassandra/production"
    And the payload should have "tag" set to "monthly-full"
    And the payload should have "schedule" set to true
    And the payload should have "scheduleExpr" set to "0 0 1 * *"
    And the CR status should have condition "Ready" with status "True"
    And the CR status should have a non-empty "syncedRepairID"

  Scenario: Create a fully configured scheduled repair
    Given an AxonOpsScheduledRepair CR with tag "full-config"
    And scheduleExpression "0 2 * * 0"
    And keyspace "analytics"
    And blacklistedTables ["large_events", "raw_logs"]
    And specificDataCenters ["dc1", "dc2"]
    And parallelism "DC-Aware"
    And segmented true with segmentsPerNode 4
    And incremental true
    And jobThreads 2
    And primaryRange false
    And optimiseStreams true
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "keyspace" set to "analytics"
    And the API payload should have "blacklistedTables" set to ["large_events", "raw_logs"]
    And the API payload should have "specificDataCenters" set to ["dc1", "dc2"]
    And the API payload should have "parallelism" set to "DC-Aware"
    And the API payload should have "segmented" set to true
    And the API payload should have "segmentsPerNode" set to 4
    And the API payload should have "incremental" set to true
    And the API payload should have "jobThreads" set to 2
    And the API payload should have "optimiseStreams" set to true

  # --- Paxos options ---

  Scenario: Scheduled repair with Skip Paxos
    Given an AxonOpsScheduledRepair CR with tag "skip-paxos"
    And skipPaxos true
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "paxos" set to "Skip Paxos"
    And the API payload should have "skipPaxos" set to true
    And the API payload should have "paxosOnly" set to false

  Scenario: Scheduled repair with Paxos Only
    Given an AxonOpsScheduledRepair CR with tag "paxos-only"
    And paxosOnly true
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "paxos" set to "Paxos Only"
    And the API payload should have "paxosOnly" set to true
    And the API payload should have "skipPaxos" set to false

  Scenario: Default paxos mode when neither flag is set
    Given an AxonOpsScheduledRepair CR with tag "default-paxos"
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "paxos" set to "Default"

  Scenario: SkipPaxos and PaxosOnly are mutually exclusive
    Given an AxonOpsScheduledRepair CR with tag "invalid-paxos"
    And skipPaxos true
    And paxosOnly true
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the CR status should have condition "Failed" with reason "ValidationError"

  # --- Update lifecycle ---

  Scenario: Update repair triggers delete and recreate
    Given an AxonOpsScheduledRepair CR with tag "updatable" that is Ready
    And the repair exists in AxonOps with ID "existing-repair-uuid"
    When I update the scheduleExpression from "0 0 1 * *" to "0 0 * * 0"
    And the operator reconciles the AxonOpsScheduledRepair CR
    Then the AxonOps API should receive a DELETE to "/api/v1/cassandrascheduledrepair/{org}/cassandra/production?id=existing-repair-uuid"
    And the AxonOps API should receive a POST with "scheduleExpr" set to "0 0 * * 0"
    And the CR status "syncedRepairID" should be updated to the new ID

  # --- Deletion ---

  Scenario: Delete repair removes it from AxonOps
    Given an AxonOpsScheduledRepair CR with tag "deletable" that is Ready
    And the repair has synced ID "delete-repair-uuid"
    When I delete the AxonOpsScheduledRepair CR
    And the operator reconciles the deletion
    Then the AxonOps API should receive a DELETE with id query param "delete-repair-uuid"
    And the finalizer should be removed
    And the CR should no longer exist

  Scenario: Delete is idempotent when repair already removed
    Given an AxonOpsScheduledRepair CR with tag "already-gone" that is Ready
    And the AxonOps API returns 404 for the delete request
    When I delete the AxonOpsScheduledRepair CR
    And the operator reconciles the deletion
    Then the finalizer should be removed
    And the CR should no longer exist

  # --- Error handling ---

  Scenario: Connection not found sets Failed condition
    Given an AxonOpsScheduledRepair CR referencing connection "nonexistent"
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the CR status should have condition "Failed" with reason "FailedToResolveConnection"

  Scenario: API error sets Failed condition with requeue
    Given an AxonOpsScheduledRepair CR with valid connection
    And the AxonOps API returns 500 Internal Server Error on POST
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the CR status should have condition "Failed" with reason "SyncFailed"
    And the reconciliation should be requeued after 30 seconds

  # --- Idempotency ---

  Scenario: No API calls when repair is already synced
    Given an AxonOpsScheduledRepair CR that is Ready with observedGeneration matching
    And syncedRepairID is set
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then no API calls should be made to AxonOps

  # --- Parallelism modes ---

  Scenario: Sequential parallelism mode
    Given an AxonOpsScheduledRepair CR with tag "sequential"
    And parallelism "Sequential"
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "parallelism" set to "Sequential"

  Scenario: Parallel parallelism mode (default)
    Given an AxonOpsScheduledRepair CR with tag "parallel-default"
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "parallelism" set to "Parallel"

  # --- Targeting ---

  Scenario: Repair targeting specific nodes
    Given an AxonOpsScheduledRepair CR with tag "node-targeted"
    And nodes ["node-1", "node-2"]
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "nodes" set to ["node-1", "node-2"]

  Scenario: Repair targeting specific tables
    Given an AxonOpsScheduledRepair CR with tag "table-targeted"
    And keyspace "my_app"
    And tables ["users", "orders"]
    When the operator reconciles the AxonOpsScheduledRepair CR
    Then the API payload should have "keyspace" set to "my_app"
    And the API payload should have "tables" set to ["users", "orders"]
