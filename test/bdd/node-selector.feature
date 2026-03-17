Feature: NodeSelector support for AxonOpsServer components
  As a cluster administrator
  I need to constrain component pods to specific nodes using nodeSelector
  So that workloads run on appropriate hardware or in specific topology zones

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: TimeSeries pods scheduled on nodes matching nodeSelector
    Given an AxonOpsServer CR with TimeSeries nodeSelector:
      | key      | value |
      | disktype | ssd   |
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet pod template should have nodeSelector "disktype" set to "ssd"

  Scenario: Search pods scheduled on nodes matching nodeSelector
    Given an AxonOpsServer CR with Search nodeSelector:
      | key      | value |
      | disktype | ssd   |
    When the operator reconciles the AxonOpsServer CR
    Then the Search StatefulSet pod template should have nodeSelector "disktype" set to "ssd"

  Scenario: Server pods scheduled on nodes matching nodeSelector
    Given an AxonOpsServer CR with Server nodeSelector:
      | key  | value |
      | role | infra |
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet pod template should have nodeSelector "role" set to "infra"

  Scenario: Dashboard pods scheduled on nodes matching nodeSelector
    Given an AxonOpsServer CR with Dashboard nodeSelector:
      | key  | value    |
      | role | frontend |
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard Deployment pod template should have nodeSelector "role" set to "frontend"

  Scenario: Multiple nodeSelector labels on a single component
    Given an AxonOpsServer CR with TimeSeries nodeSelector:
      | key                              | value      |
      | disktype                         | ssd        |
      | topology.kubernetes.io/zone      | us-east-1a |
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet pod template should have nodeSelector "disktype" set to "ssd"
    And the TimeSeries StatefulSet pod template should have nodeSelector "topology.kubernetes.io/zone" set to "us-east-1a"

  Scenario: Different nodeSelectors per component
    Given an AxonOpsServer CR with:
      | component   | nodeSelector_key | nodeSelector_value |
      | timeSeries  | disktype         | ssd                |
      | search      | disktype         | ssd                |
      | server      | role             | infra              |
      | dashboard   | role             | frontend           |
    When the operator reconciles the AxonOpsServer CR
    Then each component workload should have its own nodeSelector applied independently

  Scenario: No nodeSelector when not specified
    Given an AxonOpsServer CR with no nodeSelector on any component
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet pod template should have no nodeSelector
    And the Search StatefulSet pod template should have no nodeSelector
    And the Server StatefulSet pod template should have no nodeSelector
    And the Dashboard Deployment pod template should have no nodeSelector

  Scenario: Update nodeSelector triggers pod rescheduling
    Given an AxonOpsServer CR with TimeSeries nodeSelector "disktype" set to "ssd"
    And the TimeSeries StatefulSet is running
    When I update the TimeSeries nodeSelector to "disktype" set to "nvme"
    And the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet pod template should have nodeSelector "disktype" set to "nvme"

  Scenario: Remove nodeSelector from component
    Given an AxonOpsServer CR with Server nodeSelector "role" set to "infra"
    And the Server StatefulSet is running with nodeSelector
    When I remove the nodeSelector from the Server component
    And the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet pod template should have no nodeSelector
