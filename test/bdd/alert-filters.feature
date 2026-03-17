Feature: AxonOpsMetricAlert filter configuration
  As an AxonOps user
  I need to apply filters to metric alerts
  So that alerts target specific data centers, racks, hosts, or other dimensions

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And an AxonOpsConnection "prod-connection" exists with valid credentials
    And the AxonOps API is reachable

  Scenario: Create metric alert with DataCenter filter
    Given an AxonOpsMetricAlert CR with filter:
      | filter     | value    |
      | DataCenter | us-east-1|
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include filter "DataCenter" with value "us-east-1"
    And the AxonOpsMetricAlert status should be "Ready"

  Scenario: Create metric alert with Rack filter
    Given an AxonOpsMetricAlert CR with filter:
      | filter | value |
      | Rack   | rack1 |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include filter "Rack" with value "rack1"

  Scenario: Create metric alert with HostID filter
    Given an AxonOpsMetricAlert CR with filter:
      | filter | value                                |
      | HostID | 550e8400-e29b-41d4-a716-446655440000 |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include filter "HostID" with value "550e8400-e29b-41d4-a716-446655440000"

  Scenario: Create metric alert with multiple filters
    Given an AxonOpsMetricAlert CR with filters:
      | filter     | value     |
      | DataCenter | us-east-1 |
      | Rack       | rack1     |
      | Keyspace   | my_ks     |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include all three filters

  Scenario: Create metric alert with Scope filter
    Given an AxonOpsMetricAlert CR with filter:
      | filter | value  |
      | Scope  | node   |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include filter "Scope" with value "node"

  Scenario: Create metric alert with Percentile filter
    Given an AxonOpsMetricAlert CR with filter:
      | filter     | value |
      | Percentile | p99   |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include filter "Percentile" with value "p99"

  Scenario: Create metric alert with annotations
    Given an AxonOpsMetricAlert CR with annotations:
      | key         | value                               |
      | summary     | CPU usage exceeded threshold        |
      | description | CPU usage on {{ $labels.instance }} |
      | widgetUrl   | https://dashboard.example.com/cpu   |
    When the operator reconciles the AxonOpsMetricAlert CR
    Then the alert rule sent to AxonOps API should include all annotations
    And the AxonOpsMetricAlert status should be "Ready"
