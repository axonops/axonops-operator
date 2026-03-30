Feature: Dashboard replica configuration
  As a cluster administrator
  I need to configure the number of Dashboard replicas
  So that I can ensure high availability for the web UI

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Dashboard uses default replica count of 1
    Given an AxonOpsPlatform CR with Dashboard enabled and no replica override
    When the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard Deployment should have 1 replica

  Scenario: Dashboard with custom replica count
    Given an AxonOpsPlatform CR with Dashboard replicas set to 3
    When the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard Deployment should have 3 replicas

  Scenario: Update Dashboard replica count
    Given an AxonOpsPlatform CR with Dashboard replicas set to 1
    And the Dashboard Deployment is running with 1 replica
    When I update the Dashboard replicas to 3
    And the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard Deployment should be updated to 3 replicas

  Scenario: Scale Dashboard down to 1 replica
    Given an AxonOpsPlatform CR with Dashboard replicas set to 3
    And the Dashboard Deployment is running with 3 replicas
    When I update the Dashboard replicas to 1
    And the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard Deployment should be updated to 1 replica
