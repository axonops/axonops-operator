Feature: Health probes for AxonOpsServer components
  As a Kubernetes operator
  I need to configure appropriate liveness, readiness, and startup probes per component
  So that Kubernetes can properly manage pod lifecycle and traffic routing

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsServer CR with all four components enabled

  Scenario: TimeSeries has liveness and readiness probes
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet container should have a liveness probe
    And the liveness probe should have a period of 30 seconds
    And the TimeSeries StatefulSet container should have a readiness probe
    And the readiness probe should have a period of 10 seconds

  Scenario: Search has startup, liveness, and readiness probes
    When the operator reconciles the AxonOpsServer CR
    Then the Search StatefulSet container should have a startup probe
    And the startup probe should allow 30 retries with 10 second period
    And the Search StatefulSet container should have a liveness probe
    And the liveness probe should have a period of 20 seconds
    And the Search StatefulSet container should have a readiness probe
    And the readiness probe should have a period of 5 seconds with 3 failure threshold

  Scenario: Server has startup, liveness, and readiness probes
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet container should have a startup probe
    And the startup probe should allow 60 retries with 2 second period
    And the Server StatefulSet container should have a liveness probe
    And the liveness probe should have a period of 10 seconds
    And the Server StatefulSet container should have a readiness probe
    And the readiness probe should have a period of 5 seconds

  Scenario: Dashboard has liveness and readiness probes
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard Deployment container should have a liveness probe
    And the liveness probe should have a period of 10 seconds
    And the Dashboard Deployment container should have a readiness probe
    And the readiness probe should have a period of 5 seconds
