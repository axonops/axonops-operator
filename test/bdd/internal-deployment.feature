Feature: Deploy all AxonOpsServer components with internal databases
  As a platform operator
  I want to deploy a complete AxonOpsServer stack with all four components (TimeSeries, Search, Server, Dashboard)
  Using internal Cassandra and OpenSearch instances managed by the operator
  So that I have a fully self-contained observability platform

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And cert-manager is available in the cluster

  Scenario: Deploy all four components with default configuration
    Given an AxonOpsServer CR with all components enabled
    And spec.timeSeries.enabled set to true
    And spec.search.enabled set to true
    And spec.server.enabled set to true
    And spec.dashboard.enabled set to true
    When the operator reconciles the AxonOpsServer CR
    Then a axondb-timeseries StatefulSet should be created
    And a axondb-search StatefulSet should be created
    And a axon-server StatefulSet should be created
    And a axon-dash Deployment should be created

  Scenario: All components have correct owner references
    Given an AxonOpsServer CR named "axon-prod" with all components enabled
    When the operator reconciles the AxonOpsServer CR
    Then the axondb-timeseries StatefulSet should be owned by the AxonOpsServer CR
    And the axondb-search StatefulSet should be owned by the AxonOpsServer CR
    And the axon-server StatefulSet should be owned by the AxonOpsServer CR
    And the axon-dash Deployment should be owned by the AxonOpsServer CR
    And all Services and Secrets should be owned by the AxonOpsServer CR

  Scenario: Status conditions reflect internal component mode
    Given an AxonOpsServer CR with all components configured for internal deployment
    When the operator reconciles the AxonOpsServer CR
    Then the AxonOpsServer status should have condition "CertManagerReady=True"
    And the status should reflect "TimeSeriesMode=Internal"
    And the status should reflect "SearchMode=Internal"
    And the status should reflect "Ready=True" once all components are running

  Scenario: Headless Services created for StatefulSet DNS
    Given an AxonOpsServer CR with axondb-timeseries and axondb-search enabled
    When the operator reconciles the AxonOpsServer CR
    Then a headless Service named "axondb-timeseries" should be created for cluster discovery
    And a headless Service named "axondb-search" should be created for cluster discovery
    And these Services should have clusterIP: None

  Scenario: ClusterIP Services created alongside headless Services
    Given an AxonOpsServer CR with all internal database components enabled
    When the operator reconciles the AxonOpsServer CR
    Then a ClusterIP Service named "axondb-timeseries" should be created
    And a ClusterIP Service named "axondb-search" should be created
    And a ClusterIP Service for axon-server agent port (1888) should be created
    And a ClusterIP Service for axon-server API port (8080) should be created
    And a ClusterIP Service for axon-dash (port 3000) should be created

  Scenario: ServiceAccounts created per component
    Given an AxonOpsServer CR with all components enabled
    When the operator reconciles the AxonOpsServer CR
    Then a ServiceAccount named "axondb-timeseries" should be created
    And a ServiceAccount named "axondb-search" should be created
    And a ServiceAccount named "axon-server" should be created
    And a ServiceAccount named "axon-dash" should be created

  Scenario: Server config Secret contains correct database URLs
    Given an AxonOpsServer CR named "axon-prod" with internal TimeSeries and Search enabled
    And the namespace is "default"
    When the operator reconciles the AxonOpsServer CR
    Then a Secret containing the server configuration should be created
    And the config should reference "axondb-timeseries.default.svc.cluster.local" for TimeSeries host
    And the config should reference "axondb-search.default.svc.cluster.local:9200" for Search host
    And the config should include the normalized orgName in lowercase
