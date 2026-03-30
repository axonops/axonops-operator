Feature: AxonOpsPlatform components with resource cleanup
  As an operator user
  I want to enable or disable individual components in AxonOpsPlatform
  And have the operator automatically create or remove associated Kubernetes resources
  So that I can scale components up and down without manual cleanup

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Disable TimeSeries component cleans up all resources
    Given an existing AxonOpsPlatform CR with spec.timeSeries.enabled set to true
    And the axondb-timeseries StatefulSet, Services, Secrets, and TLS Certificate are created
    When the user updates the CR to set spec.timeSeries.enabled to false
    And the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should be deleted
    And the axondb-timeseries headless Service should be deleted
    And the axondb-timeseries ClusterIP Service should be deleted
    And the axondb-timeseries auth Secret should be deleted
    And the axondb-timeseries TLS Certificate should be deleted
    And the axondb-timeseries keystore Secret should be deleted

  Scenario: Disable Search component cleans up all resources
    Given an existing AxonOpsPlatform CR with spec.search.enabled set to true
    And the axondb-search StatefulSet, Services, Secrets, and TLS Certificate are created
    When the user updates the CR to set spec.search.enabled to false
    And the operator reconciles the AxonOpsPlatform CR
    Then the axondb-search StatefulSet should be deleted
    And the axondb-search headless Service should be deleted
    And the axondb-search ClusterIP Service should be deleted
    And the axondb-search auth Secret should be deleted
    And the axondb-search TLS Certificate should be deleted
    And the axondb-search keystore Secret should be deleted

  Scenario: Re-enable disabled TimeSeries component recreates resources
    Given an AxonOpsPlatform CR with spec.timeSeries.enabled currently set to false
    And no axondb-timeseries resources exist
    When the user updates the CR to set spec.timeSeries.enabled to true
    And the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should be created
    And the axondb-timeseries Services should be created
    And the axondb-timeseries Secrets should be created
    And the axondb-timeseries TLS Certificate should be created
    And the axon-server config should be updated to reference the TimeSeries component

  Scenario: Disable Server component removes StatefulSet and Services
    Given an existing AxonOpsPlatform CR with spec.server.enabled set to true
    And the axon-server StatefulSet and Services are running
    When the user updates the CR to set spec.server.enabled to false
    And the operator reconciles the AxonOpsPlatform CR
    Then the axon-server StatefulSet should be deleted
    And the axon-server agent Service (port 1888) should be deleted
    And the axon-server API Service (port 8080) should be deleted
    And the axon-server config Secret should be deleted

  Scenario: Disable Dashboard component removes Deployment and Service
    Given an existing AxonOpsPlatform CR with spec.dashboard.enabled set to true
    And the axon-dash Deployment and Service are running
    When the user updates the CR to set spec.dashboard.enabled to false
    And the operator reconciles the AxonOpsPlatform CR
    Then the axon-dash Deployment should be deleted
    And the axon-dash Service should be deleted
    And the axon-dash ConfigMap should be deleted

  Scenario: AxonOpsPlatform with disabled component on initial creation
    Given an AxonOpsPlatform CR with:
      - spec.timeSeries.enabled set to false
      - spec.search.enabled set to true
      - spec.server.enabled set to true
      - spec.dashboard.enabled set to true
    When the operator reconciles the AxonOpsPlatform CR for the first time
    Then no axondb-timeseries resources should be created
    And the axondb-search StatefulSet should be created
    And the axon-server StatefulSet should be created
    And the axon-dash Deployment should be created

  Scenario: Multiple component disable transitions happen correctly
    Given an AxonOpsPlatform CR with all components enabled
    And all resources are fully created
    When the user disables TimeSeries, Search, and Dashboard in sequence
    And the operator reconciles after each update
    Then each component's resources should be deleted independently
    And the Server component should remain unaffected
    And the Server config should be updated to reflect external TimeSeries and Search endpoints (if configured)
