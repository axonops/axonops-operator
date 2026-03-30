Feature: AxonOpsPlatform components
  As a Kubernetes operator
  I need to expose the correct ports via Services for each component
  So that components can communicate and external clients can connect

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsPlatform CR with all four components enabled

  Scenario: TimeSeries exposes correct ports
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries headless Service should expose port 9042 for CQL
    And the TimeSeries headless Service should expose port 7199 for JMX
    And the TimeSeries headless Service should expose port 7000 for intra-node communication
    And the TimeSeries headless Service should expose port 7001 for TLS intra-node communication

  Scenario: Search exposes correct ports
    When the operator reconciles the AxonOpsPlatform CR
    Then the Search headless Service should expose port 9200 for HTTP
    And the Search headless Service should expose port 9300 for transport
    And the Search headless Service should expose port 9600 for metrics

  Scenario: Server exposes agent and API ports on separate Services
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server agent Service should expose port 1888
    And the Server API Service should expose port 8080
    And the Server agent Service and API Service should be separate resources

  Scenario: Dashboard exposes HTTP port
    When the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard ClusterIP Service should expose port 3000

  Scenario: Headless Services have clusterIP set to None
    When the operator reconciles the AxonOpsPlatform CR
    Then the TimeSeries headless Service should have clusterIP "None"
    And the Search headless Service should have clusterIP "None"
    And the Server headless Service should have clusterIP "None"

  Scenario: ClusterIP Services are created alongside headless Services
    When the operator reconciles the AxonOpsPlatform CR
    Then a ClusterIP Service should exist for TimeSeries
    And a ClusterIP Service should exist for Search
    And the Dashboard Service should be of type ClusterIP
