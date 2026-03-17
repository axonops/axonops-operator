Feature: Resource requests and limits for AxonOpsServer components
  As a cluster administrator
  I need to configure CPU and memory requests and limits per component
  So that I can ensure proper resource allocation and prevent resource starvation

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Custom resource requests and limits for TimeSeries
    Given an AxonOpsServer CR with TimeSeries resources:
      | resource | requests | limits |
      | cpu      | 500m     | 2000m  |
      | memory   | 1Gi      | 4Gi    |
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries StatefulSet container should have CPU request "500m"
    And the TimeSeries StatefulSet container should have CPU limit "2000m"
    And the TimeSeries StatefulSet container should have memory request "1Gi"
    And the TimeSeries StatefulSet container should have memory limit "4Gi"

  Scenario: Custom resource requests and limits for Search
    Given an AxonOpsServer CR with Search resources:
      | resource | requests | limits |
      | cpu      | 1000m    | 4000m  |
      | memory   | 2Gi      | 8Gi    |
    When the operator reconciles the AxonOpsServer CR
    Then the Search StatefulSet container should have CPU request "1000m"
    And the Search StatefulSet container should have CPU limit "4000m"
    And the Search StatefulSet container should have memory request "2Gi"
    And the Search StatefulSet container should have memory limit "8Gi"

  Scenario: Custom resource requests and limits for Server
    Given an AxonOpsServer CR with Server resources:
      | resource | requests | limits |
      | cpu      | 250m     | 1000m  |
      | memory   | 512Mi    | 2Gi    |
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet container should have CPU request "250m"
    And the Server StatefulSet container should have CPU limit "1000m"
    And the Server StatefulSet container should have memory request "512Mi"
    And the Server StatefulSet container should have memory limit "2Gi"

  Scenario: Custom resource requests and limits for Dashboard
    Given an AxonOpsServer CR with Dashboard resources:
      | resource | requests | limits |
      | cpu      | 100m     | 500m   |
      | memory   | 128Mi    | 512Mi  |
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard Deployment container should have CPU request "100m"
    And the Dashboard Deployment container should have CPU limit "500m"
    And the Dashboard Deployment container should have memory request "128Mi"
    And the Dashboard Deployment container should have memory limit "512Mi"

  Scenario: Custom heap size for TimeSeries
    Given an AxonOpsServer CR with TimeSeries heapSize "2048M"
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should have environment variable "CASSANDRA_HEAP_SIZE" set to "2048M"

  Scenario: Custom heap size for Search
    Given an AxonOpsServer CR with Search heapSize "4g"
    When the operator reconciles the AxonOpsServer CR
    Then the Search container should have environment variable "OPENSEARCH_HEAP_SIZE" set to "4g"

  Scenario: Default heap sizes when not specified
    Given an AxonOpsServer CR with no heap size overrides
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should have environment variable "CASSANDRA_HEAP_SIZE" set to "1024M"
    And the Search container should have environment variable "OPENSEARCH_HEAP_SIZE" set to "2g"
