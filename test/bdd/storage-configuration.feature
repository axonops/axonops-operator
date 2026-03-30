Feature: AxonOpsPlatform components
  As a cluster administrator
  I need to configure persistent volume sizes and storage classes per component
  So that I can size storage appropriately for my workload

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: TimeSeries uses default storage size of 10Gi
    Given an AxonOpsPlatform CR with TimeSeries enabled and no storage overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet should have a VolumeClaimTemplate "data"
    And the VolumeClaimTemplate should request "10Gi" storage
    And the VolumeClaimTemplate should use AccessMode "ReadWriteOnce"
    And the volume should be mounted at "/var/lib/cassandra"

  Scenario: Search uses default storage size of 10Gi
    Given an AxonOpsPlatform CR with Search enabled and no storage overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet should have a VolumeClaimTemplate "data"
    And the VolumeClaimTemplate should request "10Gi" storage
    And the VolumeClaimTemplate should use AccessMode "ReadWriteOnce"
    And the volume should be mounted at "/usr/share/opensearch/data"

  Scenario: Server uses default storage size of 1Gi
    Given an AxonOpsPlatform CR with Server enabled and no storage overrides
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server StatefulSet should have a VolumeClaimTemplate "data"
    And the VolumeClaimTemplate should request "1Gi" storage
    And the VolumeClaimTemplate should use AccessMode "ReadWriteOnce"
    And the volume should be mounted at "/var/lib/axonops"

  Scenario: Custom storage size for TimeSeries
    Given an AxonOpsPlatform CR with TimeSeries storageConfig requesting "50Gi"
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet VolumeClaimTemplate should request "50Gi" storage

  Scenario: Custom storage size for Search
    Given an AxonOpsPlatform CR with Search storageConfig requesting "30Gi"
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search StatefulSet VolumeClaimTemplate should request "30Gi" storage

  Scenario: Custom storage size for Server
    Given an AxonOpsPlatform CR with Server storageConfig requesting "5Gi"
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server StatefulSet VolumeClaimTemplate should request "5Gi" storage

  Scenario: Custom storage class for TimeSeries
    Given an AxonOpsPlatform CR with TimeSeries storageConfig using storageClassName "fast-ssd"
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries StatefulSet VolumeClaimTemplate should use storageClassName "fast-ssd"
