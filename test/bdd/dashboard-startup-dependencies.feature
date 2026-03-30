Feature: Dashboard startup waits for server to be Ready
  As an operator user
  I want the dashboard to start only after the server is Ready
  So that the dashboard can successfully connect to the server API on startup

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Dashboard waits for server to be Ready
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image | axonops/axondb-timeseries:latest |
      | spec.search.image     | axonops/axondb-search:latest     |
      | spec.dashboard.enabled| true                             |
    When the operator begins reconciliation
    And the axon-server StatefulSet is not yet Ready
    Then the axon-dash Deployment should not be created
    And the AxonOpsPlatform status should show "Waiting for server to be ready"
    When the axon-server StatefulSet becomes Ready
    Then the axon-dash Deployment should be created
    And reconciliation should complete successfully

  Scenario: Dashboard skips server wait when server is disabled
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image  | axonops/axondb-timeseries:latest |
      | spec.search.image      | axonops/axondb-search:latest     |
      | spec.dashboard.enabled | true                             |
      | spec.server.enabled    | false                            |
    When the operator begins reconciliation
    Then the axon-dash Deployment should not be created
    And the status should indicate server is not enabled

  Scenario: Dashboard starts immediately when server is external
    Given an AxonOpsPlatform CR with:
      | spec.server.external.host | server.example.com               |
      | spec.dashboard.enabled    | true                             |
    When the operator begins reconciliation
    Then the axon-dash Deployment should be created immediately
    And no wait for server readiness is required

  Scenario: Dashboard remains stable if server becomes unavailable after startup
    Given an AxonOpsPlatform CR with:
      | spec.timeseries.image | axonops/axondb-timeseries:latest |
      | spec.search.image     | axonops/axondb-search:latest     |
      | spec.dashboard.enabled| true                             |
    And the axon-server StatefulSet is Ready
    And the axon-dash Deployment has been running
    When the axon-server StatefulSet becomes NotReady
    Then the axon-dash Deployment should continue running
    And reconciliation should maintain current state
    When the axon-server StatefulSet recovers to Ready
    Then reconciliation should complete successfully

  Scenario: Status conditions reflect dashboard dependency waiting state
    Given an AxonOpsPlatform CR with server not yet ready and dashboard enabled
    When the operator reconciles
    Then the AxonOpsPlatform.status.conditions should include:
      | Type    | Status | Reason          | Message                         |
      | Ready   | False  | WaitingForDeps   | Waiting for dependencies to be ready |
      | Dashboard | False  | WaitingForServer | Dashboard waiting for server to be ready |

  Scenario: Dashboard container environment is configured with server endpoints
    Given an AxonOpsPlatform CR with ready server and dashboard enabled
    When the axon-dash Deployment is created
    Then the dashboard container should have environment variables pointing to:
      | AXONOPS_SERVER_HOST | axon-server.<namespace>.svc.cluster.local |
      | AXONOPS_SERVER_PORT | 8080                                       |
    And the deployment should have readiness/liveness probes configured appropriately

  Scenario: Dashboard respects dashboard.enabled flag
    Given an AxonOpsPlatform CR with:
      | spec.dashboard.enabled | false |
    When the operator begins reconciliation
    Then the axon-dash Deployment should not be created
    And no dashboard resources should be created or managed
