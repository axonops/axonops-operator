Feature: AxonOpsSilenceWindow alert silence management
  As a cluster administrator
  I need to declaratively manage alert silence windows via Kubernetes CRDs
  So that maintenance windows suppress notifications automatically

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsConnection "test-connection" exists with valid API credentials
    And a cluster "production" is registered in AxonOps

  # --- One-shot silences ---

  Scenario: Create a one-shot silence window
    Given an AxonOpsSilenceWindow CR:
      | field          | value            |
      | connectionRef  | test-connection  |
      | clusterName    | production       |
      | clusterType    | cassandra        |
      | duration       | 1h               |
      | recurring      | false            |
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the AxonOps API should receive a POST to "/api/v1/silenceWindow/{org}/cassandra/production"
    And the payload should have "Active" set to true
    And the payload should have "IsRecurring" set to false
    And the payload should have "Duration" set to "1h"
    And the payload should have a non-empty "ID" (UUID)
    And the CR status should have condition "Ready" with status "True"

  Scenario: Create a one-shot silence targeting specific data centers
    Given an AxonOpsSilenceWindow CR with duration "2h"
    And datacenters ["dc1", "dc2"]
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the API payload should have "DCs" set to ["dc1", "dc2"]

  # --- Recurring silences ---

  Scenario: Create a recurring silence with cron schedule
    Given an AxonOpsSilenceWindow CR:
      | field          | value            |
      | connectionRef  | test-connection  |
      | clusterName    | production       |
      | clusterType    | cassandra        |
      | duration       | 3h               |
      | recurring      | true             |
      | cronExpression | 0 1 * * 0        |
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the API payload should have "IsRecurring" set to true
    And the API payload should have "CronExpr" set to "0 1 * * 0"
    And the API payload should have "Duration" set to "3h"

  Scenario: Create a recurring nightly maintenance silence
    Given an AxonOpsSilenceWindow CR with duration "4h"
    And recurring true
    And cronExpression "0 22 * * *"
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the API payload should have "CronExpr" set to "0 22 * * *"
    And the API payload should have "IsRecurring" set to true

  # --- Lifecycle ---

  Scenario: Update silence triggers delete and recreate
    Given an AxonOpsSilenceWindow CR with duration "1h" that is Ready
    And the silence exists in AxonOps with ID "existing-uuid"
    When I update the duration from "1h" to "2h"
    And the operator reconciles the AxonOpsSilenceWindow CR
    Then the AxonOps API should receive a DELETE to "/api/v1/silenceWindow/{org}/cassandra/production/existing-uuid"
    And the AxonOps API should receive a POST with "Duration" set to "2h"

  Scenario: Delete silence removes it from AxonOps
    Given an AxonOpsSilenceWindow CR that is Ready
    And the silence has synced ID "delete-uuid"
    When I delete the AxonOpsSilenceWindow CR
    And the operator reconciles the deletion
    Then the AxonOps API should receive a DELETE with the synced ID
    And the finalizer should be removed
    And the CR should no longer exist

  Scenario: Delete is idempotent when silence already removed
    Given an AxonOpsSilenceWindow CR that is Ready
    And the AxonOps API returns 404 for the delete request
    When I delete the AxonOpsSilenceWindow CR
    And the operator reconciles the deletion
    Then the finalizer should be removed
    And the CR should no longer exist

  # --- Error handling ---

  Scenario: Connection not found sets Failed condition
    Given an AxonOpsSilenceWindow CR referencing connection "nonexistent"
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the CR status should have condition "Failed" with reason "FailedToResolveConnection"

  Scenario: API error sets Failed condition with requeue
    Given an AxonOpsSilenceWindow CR with valid connection
    And the AxonOps API returns 500 Internal Server Error
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then the CR status should have condition "Failed" with reason "SyncFailed"
    And the reconciliation should be requeued after 30 seconds

  # --- Idempotency ---

  Scenario: No API calls when silence is already synced
    Given an AxonOpsSilenceWindow CR that is Ready with observedGeneration matching
    And syncedSilenceID is set
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then no API calls should be made to AxonOps

  # --- Active flag ---

  Scenario: Inactive silence is not created
    Given an AxonOpsSilenceWindow CR with active set to false
    When the operator reconciles the AxonOpsSilenceWindow CR
    Then no POST should be sent to the AxonOps API
    And the CR status should have condition "Ready" with status "True"

  Scenario: Deactivating an active silence deletes it
    Given an AxonOpsSilenceWindow CR with active true that is Ready
    And the silence has synced ID "active-uuid"
    When I update active to false
    And the operator reconciles the AxonOpsSilenceWindow CR
    Then the AxonOps API should receive a DELETE with the synced ID
