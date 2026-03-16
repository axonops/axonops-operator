Feature: AxonOpsHealthcheckTCP lifecycle for TCP port monitoring
  As an operator user
  I want to define TCP port healthchecks using AxonOpsHealthcheckTCP CRs
  That verify network services are listening on expected ports
  So that I can detect when critical services become unavailable

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create TCP healthcheck for cassandra port
    Given an AxonOpsHealthcheckTCP CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.tcp set to "cassandra.example.com:9042"
    When the operator reconciles the AxonOpsHealthcheckTCP CR
    Then the AxonOps API should be called to create a TCP healthcheck
    And the AxonOpsHealthcheckTCP status should have field "syncedCheckID"
    And the status condition should show "Ready=True"

  Scenario: Update TCP healthcheck interval
    Given an existing AxonOpsHealthcheckTCP CR with syncedCheckID in status
    When the user updates the CR to change spec.interval from "60" to "30"
    And the operator reconciles the AxonOpsHealthcheckTCP CR
    Then the AxonOps API should be called with HTTP PUT to update the interval
    And the syncedCheckID should remain unchanged
    And the healthcheck should run with the new interval

  Scenario: Delete TCP healthcheck
    Given an existing AxonOpsHealthcheckTCP CR with syncedCheckID in status
    When the user deletes the AxonOpsHealthcheckTCP CR
    Then the operator should call the AxonOps API to delete the healthcheck
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: TCP healthcheck for multiple ports
    Given multiple AxonOpsHealthcheckTCP CRs for different services:
      - cassandra.prod:9042
      - opensearch.prod:9200
      - redis.prod:6379
    When the operator reconciles all healthchecks
    Then each TCP healthcheck should be created independently
    And all should have syncedCheckID in their status
    And all should show "Ready=True"

  Scenario: Missing AxonOpsConnection reference
    Given an AxonOpsHealthcheckTCP CR with spec.connectionRef set to "nonexistent"
    When the operator reconciles the AxonOpsHealthcheckTCP CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "ConnectionNotFound"
    And the healthcheck should not be synced to the AxonOps API
