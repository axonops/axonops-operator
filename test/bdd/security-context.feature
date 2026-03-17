Feature: Security context configuration for AxonOpsServer components
  As a Kubernetes operator
  I need to configure proper security contexts for all component pods
  So that pods run with least-privilege and meet security requirements

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsServer CR with all four components enabled

  Scenario: TimeSeries runs with correct user and group
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet pod should have FSGroup 999
    And the TimeSeries container should run as user 999

  Scenario: Search runs with correct user and group
    When the operator reconciles the AxonOpsServer CR
    Then the Search StatefulSet pod should have FSGroup 999
    And the Search container should run as user 999

  Scenario: Search container has IPC_LOCK capability
    When the operator reconciles the AxonOpsServer CR
    Then the Search container should have the "IPC_LOCK" capability added
    Because OpenSearch requires memory locking for performance

  Scenario: Search has init container for filesystem ownership
    When the operator reconciles the AxonOpsServer CR
    Then the Search StatefulSet should have an init container
    And the init container should chown the data directory to 999:999

  Scenario: Server runs with correct user and group
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet pod should run as user 9988
    And the Server StatefulSet pod should run as group 9988

  Scenario: Server has init container for filesystem ownership
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet should have an init container
    And the init container should chown the data directory to 9988:9988

  Scenario: Server container drops ALL capabilities
    When the operator reconciles the AxonOpsServer CR
    Then the Server container security context should drop "ALL" capabilities
