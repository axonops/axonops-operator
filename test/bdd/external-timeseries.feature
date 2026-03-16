Feature: Support external TimeSeries (Cassandra) database
  As an operator user, I want to configure AxonOpsServer to use an external Cassandra cluster
  for TimeSeries storage instead of having the operator deploy its own axondb-timeseries StatefulSet

  Scenario: Deploy with external Cassandra (no internal StatefulSet)
    Given an AxonOpsServer CR with spec.timeSeries.enabled set to true
    And spec.timeSeries.external.hosts set to ["cassandra-node1.example.com", "cassandra-node2.example.com"]
    And spec.timeSeries.authentication.username set to "axonops"
    And spec.timeSeries.authentication.secretRef referencing a Secret with the password
    When the operator reconciles the AxonOpsServer CR
    Then no axondb-timeseries StatefulSet should be created
    And no axondb-timeseries headless Service should be created
    And no axondb-timeseries ClusterIP Service should be created
    And no auto-generated TimeSeries auth Secret should be created
    And the axon-server StatefulSet should be configured to connect to the external hosts
    And the axon-server config should use the provided authentication credentials
    And the AxonOpsServer status should reflect "TimeSeries: External"

  Scenario: External Cassandra with TLS enabled
    Given an AxonOpsServer CR with spec.timeSeries.external.hosts set to ["cassandra.prod.internal:9042"]
    And spec.timeSeries.external.tls.enabled set to true
    And spec.timeSeries.external.tls.insecureSkipVerify set to false
    When the operator reconciles the AxonOpsServer CR
    Then the axon-server should be configured with TLS settings for TimeSeries connections
    And no self-signed TLS certificates should be generated for TimeSeries
    And the AxonOpsServer status condition should be "Ready=True"

  Scenario: Switch from internal to external TimeSeries
    Given an existing AxonOpsServer with an internal axondb-timeseries StatefulSet running
    When the user updates the CR to set spec.timeSeries.external.hosts to ["external-cassandra:9042"]
    Then the operator should remove the internal axondb-timeseries StatefulSet
    And the operator should remove the associated Services and Secrets
    And the axon-server should be reconfigured to point to the external hosts
    And no data loss warning should be surfaced in status conditions

  Scenario: External Cassandra becomes unreachable
    Given an AxonOpsServer CR configured with external TimeSeries hosts
    And the external Cassandra cluster is unreachable
    When the operator reconciles the AxonOpsServer CR
    Then the AxonOpsServer status should reflect a degraded condition
    And the condition message should indicate TimeSeries connectivity failure
    And the operator should not attempt to deploy an internal StatefulSet as fallback

  Scenario: Missing authentication for external Cassandra
    Given an AxonOpsServer CR with spec.timeSeries.external.hosts configured
    And no authentication credentials provided (no username, no secretRef)
    When the operator reconciles the AxonOpsServer CR
    Then the operator should set a status condition indicating missing credentials
    And reconciliation should not proceed for the server component
