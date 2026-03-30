Feature: Server startup waits for timeseries and search dependencies
  As an operator user
  I want the server to start only after timeseries and search are Ready
  So that the server has healthy dependencies available on startup

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Server waits for timeseries and search to be Ready
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image | axonops/axondb-timeseries:latest |
      | spec.search.image     | axonops/axondb-search:latest     |
    When the operator begins reconciliation
    And the axondb-timeseries StatefulSet is not yet Ready
    And the axondb-search StatefulSet is not yet Ready
    Then the axon-server StatefulSet should not be created
    And the AxonOpsPlatform status should show "Waiting for timeseries to be ready"
    When the axondb-timeseries StatefulSet becomes Ready
    Then the axon-server StatefulSet should still not be created
    And the status should show "Waiting for search to be ready"
    When the axondb-search StatefulSet becomes Ready
    Then the axon-server StatefulSet should be created
    And reconciliation should complete successfully

  Scenario: Server starts immediately when timeseries is external
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.external.hosts | timeseries.example.com |
      | spec.search.image              | axonops/axondb-search:latest |
    When the operator begins reconciliation
    And the axondb-search StatefulSet is not yet Ready
    Then the axon-server StatefulSet should not be created
    And the status should show "Waiting for search to be ready"
    When the axondb-search StatefulSet becomes Ready
    Then the axon-server StatefulSet should be created
    And no wait for timeseries is required

  Scenario: Server starts immediately when search is external
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image      | axonops/axondb-timeseries:latest |
      | spec.search.external.hosts | search.example.com |
    When the operator begins reconciliation
    And the axondb-timeseries StatefulSet is not yet Ready
    Then the axon-server StatefulSet should not be created
    And the status should show "Waiting for timeseries to be ready"
    When the axondb-timeseries StatefulSet becomes Ready
    Then the axon-server StatefulSet should be created
    And no wait for search is required

  Scenario: Server starts immediately when both timeseries and search are external
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.external.hosts | timeseries.example.com |
      | spec.search.external.hosts     | search.example.com |
    When the operator begins reconciliation
    Then the axon-server StatefulSet should be created immediately
    And no dependency waiting occurs

  Scenario: Server startup resumes if dependencies recover after becoming unavailable
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image | axonops/axondb-timeseries:latest |
      | spec.search.image     | axonops/axondb-search:latest     |
    And both timeseries and search StatefulSets are Ready
    And the axon-server StatefulSet has been running
    When the axondb-timeseries StatefulSet becomes NotReady
    Then the axon-server StatefulSet should continue running
    And reconciliation should maintain current state
    When the axondb-timeseries StatefulSet recovers to Ready
    Then reconciliation should complete successfully

  Scenario: Status conditions reflect dependency waiting state
    Given an AxonOpsPlatform CR with dependencies not yet ready
    When the operator reconciles
    Then the AxonOpsPlatform.status.conditions should include:
      | Type    | Status | Reason          | Message                         |
      | Ready   | False  | WaitingForDeps   | Waiting for dependencies to be ready |
      | Timeseries | False  | NotReady        | Timeseries StatefulSet not ready |
      | Search  | False  | NotReady        | Search StatefulSet not ready     |

  Scenario: Server deployment uses dependencies as readiness probe guards
    Given an AxonOpsPlatform CR with ready dependencies
    When the axon-server StatefulSet is created
    Then it should reference the timeseries and search service endpoints
    And the server container should have environment variables pointing to those endpoints
    And it should have readiness/liveness probes configured appropriately
