Feature: AxonOpsServer deletion and finalizer lifecycle
  As a Kubernetes operator
  I need to properly clean up all resources when an AxonOpsServer CR is deleted
  So that no orphaned resources remain in the cluster

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And cert-manager CRDs are available in the cluster
    And a namespace "axonops-test" exists

  Scenario: Finalizer is added on initial reconciliation
    When I create an AxonOpsServer CR with default configuration
    Then the AxonOpsServer CR should have the finalizer "core.axonops.com/finalizer"

  Scenario: Deleting AxonOpsServer cleans up TLS secrets
    Given an AxonOpsServer CR with internal TimeSeries and Search enabled
    And TLS Certificate resources have been created for TimeSeries and Search
    And cert-manager has issued TLS secrets for both components
    When I delete the AxonOpsServer CR
    Then the TLS secrets for TimeSeries should be deleted
    And the TLS secrets for Search should be deleted
    And the keystore password secrets should be deleted
    And the finalizer should be removed
    And the AxonOpsServer CR should be fully deleted

  Scenario: Deleting AxonOpsServer cleans up auth secrets
    Given an AxonOpsServer CR with auto-generated credentials
    And auth secrets exist for TimeSeries and Search components
    When I delete the AxonOpsServer CR
    Then the TimeSeries auth secret should be garbage collected via owner references
    And the Search auth secret should be garbage collected via owner references

  Scenario: Deleting AxonOpsServer with external databases
    Given an AxonOpsServer CR configured with external TimeSeries and Search
    And no internal TLS certificates exist
    When I delete the AxonOpsServer CR
    Then the finalizer should be removed without TLS cleanup errors
    And the AxonOpsServer CR should be fully deleted

  Scenario: Deleting AxonOpsServer removes ClusterIssuer only if auto-created
    Given an AxonOpsServer CR using the default self-signed ClusterIssuer
    And the ClusterIssuer "axonops-selfsigned" was auto-created by the operator
    When I delete the AxonOpsServer CR
    Then the auto-created ClusterIssuer should remain for other AxonOpsServer CRs
    And the finalizer should be removed

  Scenario: Owner references enable cascading deletion of child resources
    Given an AxonOpsServer CR with all four components enabled
    And all child resources have owner references pointing to the AxonOpsServer CR
    When I delete the AxonOpsServer CR
    Then Kubernetes garbage collection should delete all owned StatefulSets
    And Kubernetes garbage collection should delete all owned Deployments
    And Kubernetes garbage collection should delete all owned Services
    And Kubernetes garbage collection should delete all owned ConfigMaps
    And Kubernetes garbage collection should delete all owned ServiceAccounts
