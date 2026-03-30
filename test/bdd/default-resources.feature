Feature: Default resource requests and limits for AxonOpsPlatform components
  As a cluster administrator
  I want CPU and memory resources to be set automatically when not specified
  So that components have safe resource guarantees without requiring explicit configuration

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  # --- TimeSeries (Cassandra / JVM) ---
  # Default heap: 1024M → default memory = 1024 * 1.5 = 1536Mi

  Scenario: TimeSeries gets default memory derived from heap size
    Given an AxonOpsPlatform CR with TimeSeries enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet container should have memory request "1536Mi"
    And the TimeSeries StatefulSet container should have memory limit "1536Mi"

  Scenario: TimeSeries gets default CPU request with no CPU limit
    Given an AxonOpsPlatform CR with TimeSeries enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet container should have CPU request "1000m"
    And the TimeSeries StatefulSet container should have no CPU limit

  Scenario: TimeSeries default memory scales with a custom heap size
    Given an AxonOpsPlatform CR with TimeSeries heapSize "2048M" and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet container should have memory request "3072Mi"
    And the TimeSeries StatefulSet container should have memory limit "3072Mi"

  Scenario: Unparseable TimeSeries heap size falls back to default-heap-derived memory
    Given an AxonOpsPlatform CR with TimeSeries heapSize "invalid" and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet container should have memory request "1536Mi"
    And the TimeSeries StatefulSet container should have memory limit "1536Mi"

  Scenario: User-specified TimeSeries resources are not overridden by defaults
    Given an AxonOpsPlatform CR with TimeSeries resources:
      | resource | requests | limits |
      | cpu      | 500m     | 2000m  |
      | memory   | 1Gi      | 4Gi    |
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet container should have CPU request "500m"
    And the TimeSeries StatefulSet container should have CPU limit "2000m"
    And the TimeSeries StatefulSet container should have memory request "1Gi"
    And the TimeSeries StatefulSet container should have memory limit "4Gi"

  # --- Search (OpenSearch / JVM) ---
  # Default heap: 2g → default memory = 2048 * 1.5 = 3072Mi

  Scenario: Search gets default memory derived from heap size
    Given an AxonOpsPlatform CR with Search enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet container should have memory request "3072Mi"
    And the Search StatefulSet container should have memory limit "3072Mi"

  Scenario: Search gets default CPU request with no CPU limit
    Given an AxonOpsPlatform CR with Search enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet container should have CPU request "1000m"
    And the Search StatefulSet container should have no CPU limit

  Scenario: Search default memory scales with a custom heap size
    Given an AxonOpsPlatform CR with Search heapSize "4096M" and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet container should have memory request "6144Mi"
    And the Search StatefulSet container should have memory limit "6144Mi"

  Scenario: Unparseable Search heap size falls back to default-heap-derived memory
    Given an AxonOpsPlatform CR with Search heapSize "invalid" and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet container should have memory request "3072Mi"
    And the Search StatefulSet container should have memory limit "3072Mi"

  Scenario: User-specified Search resources are not overridden by defaults
    Given an AxonOpsPlatform CR with Search resources:
      | resource | requests | limits |
      | cpu      | 1000m    | 4000m  |
      | memory   | 2Gi      | 8Gi    |
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet container should have CPU request "1000m"
    And the Search StatefulSet container should have CPU limit "4000m"
    And the Search StatefulSet container should have memory request "2Gi"
    And the Search StatefulSet container should have memory limit "8Gi"

  # --- Server (axon-server / Go — no JVM heap) ---
  # Fixed defaults: memory 256Mi request / 512Mi limit; CPU 250m request / no limit

  Scenario: Server gets default memory when no resources are specified
    Given an AxonOpsPlatform CR with Server enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server StatefulSet container should have memory request "256Mi"
    And the Server StatefulSet container should have memory limit "512Mi"

  Scenario: Server gets default CPU request with no CPU limit
    Given an AxonOpsPlatform CR with Server enabled and no resource overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server StatefulSet container should have CPU request "250m"
    And the Server StatefulSet container should have no CPU limit

  Scenario: User-specified Server resources are not overridden by defaults
    Given an AxonOpsPlatform CR with Server resources:
      | resource | requests | limits |
      | cpu      | 500m     | 2000m  |
      | memory   | 512Mi    | 2Gi    |
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server StatefulSet container should have CPU request "500m"
    And the Server StatefulSet container should have CPU limit "2000m"
    And the Server StatefulSet container should have memory request "512Mi"
    And the Server StatefulSet container should have memory limit "2Gi"
