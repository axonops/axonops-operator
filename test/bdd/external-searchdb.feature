Feature: Support external SearchDB (OpenSearch/Elasticsearch)
  As an operator user
  I want to configure AxonOpsPlatform to use an external OpenSearch or Elasticsearch cluster for SearchDB
  Instead of having the operator deploy its own axondb-search StatefulSet
  So that I can use a managed OpenSearch/Elasticsearch in production environments

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Deploy with external OpenSearch (no internal StatefulSet)
    Given an AxonOpsPlatform CR with spec.search.enabled set to true
    And spec.search.external.hosts set to ["https://opensearch.example.com:9200"]
    And spec.search.authentication.username set to "admin"
    And spec.search.authentication.secretRef referencing a Secret with the password
    When the operator reconciles the AxonOpsPlatform CR
    Then no axondb-search StatefulSet should be created
    And no axondb-search headless Service should be created
    And no axondb-search ClusterIP Service should be created
    And no auto-generated Search auth Secret should be created
    And the axon-server StatefulSet should be configured to connect to the external OpenSearch hosts
    And the axon-server config should use the provided authentication credentials
    And the AxonOpsPlatform status should reflect "Search: External"

  Scenario: Deploy with external Elasticsearch
    Given an AxonOpsPlatform CR with spec.search.external.hosts set to ["https://elasticsearch.prod.internal:9200"]
    And spec.search.authentication configured via secretRef
    When the operator reconciles the AxonOpsPlatform CR
    Then the axon-server should be configured to connect to Elasticsearch
    And the behaviour should be identical to external OpenSearch (same API compatibility)
    And no internal axondb-search resources should be created

  Scenario: External SearchDB with TLS enabled
    Given an AxonOpsPlatform CR with spec.search.external.hosts set to ["https://search.prod.internal:9200"]
    And spec.search.external.tls.enabled set to true
    And spec.search.external.tls.insecureSkipVerify set to false
    When the operator reconciles the AxonOpsPlatform CR
    Then the axon-server should be configured with TLS settings for Search connections
    And no self-signed TLS certificates should be generated for Search
    And the AxonOpsPlatform status condition should be "Ready=True"

  Scenario: External SearchDB with TLS skip verify (self-signed certs)
    Given an AxonOpsPlatform CR with spec.search.external.hosts configured
    And spec.search.external.tls.enabled set to true
    And spec.search.external.tls.insecureSkipVerify set to true
    When the operator reconciles the AxonOpsPlatform CR
    Then the axon-server should connect to the external Search with TLS but skip certificate verification
    And this should be reflected in the server configuration

  Scenario: Switch from internal to external SearchDB
    Given an existing AxonOpsPlatform with an internal axondb-search StatefulSet running
    When the user updates the CR to set spec.search.external.hosts to ["https://managed-opensearch.aws:443"]
    Then the operator should remove the internal axondb-search StatefulSet
    And the operator should remove the associated Services and Secrets
    And the axon-server should be reconfigured to point to the external hosts
    And no data loss warning should be surfaced in status conditions

  Scenario: External SearchDB becomes unreachable
    Given an AxonOpsPlatform CR configured with external Search hosts
    And the external SearchDB cluster is unreachable
    When the operator reconciles the AxonOpsPlatform CR
    Then the AxonOpsPlatform status should reflect a degraded condition
    And the condition message should indicate Search connectivity failure
    And the operator should not attempt to deploy an internal StatefulSet as fallback

  Scenario: Missing authentication for external SearchDB
    Given an AxonOpsPlatform CR with spec.search.external.hosts configured
    And no authentication credentials provided
    When the operator reconciles the AxonOpsPlatform CR
    Then the operator should set a status condition indicating missing credentials
    And reconciliation should not proceed for the server component

  Scenario: Both TimeSeries and Search external
    Given an AxonOpsPlatform CR with both external TimeSeries and external Search configured
    And spec.timeSeries.external.hosts set to ["cassandra.prod:9042"]
    And spec.search.external.hosts set to ["https://opensearch.prod:9200"]
    When the operator reconciles the AxonOpsPlatform CR
    Then no axondb-timeseries or axondb-search StatefulSets should be created
    And only axon-server and axon-dash resources should be deployed
    And axon-server should be configured to connect to both external endpoints
    And the AxonOpsPlatform status should reflect "TimeSeries: External, Search: External"
