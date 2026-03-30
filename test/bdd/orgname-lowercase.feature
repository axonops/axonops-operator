Feature: OrgName must always be lowercase
  As an operator user
  I want the operator to normalize orgName to lowercase automatically
  So that AxonOps components always receive a valid lowercase org name

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Mixed-case orgName is normalized to lowercase
    Given an AxonOpsPlatform CR with spec.server.orgName set to "AxonOps"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should have env var AXON_AGENT_ORG set to "axonops"
    And the axon-server config secret should contain "org_name: axonops"

  Scenario: Uppercase orgName is normalized to lowercase
    Given an AxonOpsPlatform CR with spec.server.orgName set to "MY-ORG"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should have env var AXON_AGENT_ORG set to "my-org"
    And the axon-server config secret should contain "org_name: my-org"

  Scenario: Already lowercase orgName is unchanged
    Given an AxonOpsPlatform CR with spec.server.orgName set to "myorg"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should have env var AXON_AGENT_ORG set to "myorg"
    And the axon-server config secret should contain "org_name: myorg"

  Scenario: OrgName with spaces and capitals is normalized
    Given an AxonOpsPlatform CR with spec.server.orgName set to "My Org Name"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should have env var AXON_AGENT_ORG set to "my org name"
    And the axon-server config secret should contain "org_name: my org name"

  Scenario: OrgName normalization persists across reconciliation loops
    Given an AxonOpsPlatform CR with spec.server.orgName set to "TestOrg"
    And the operator has already reconciled the CR once
    When a reconciliation is triggered again (e.g., by a status update)
    Then the axondb-timeseries StatefulSet should still have env var AXON_AGENT_ORG set to "testorg"
    And the axon-server config secret should still contain "org_name: testorg"
    And no unnecessary updates should be made to the StatefulSet or config secret
