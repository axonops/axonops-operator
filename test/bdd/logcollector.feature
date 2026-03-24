Feature: AxonOpsLogCollector management
  As a Cassandra cluster administrator
  I need to declaratively manage log collector configurations via Kubernetes CRDs
  So that log collection settings are version-controlled and reconciled automatically

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists
    And an AxonOpsConnection "test-connection" exists with valid API credentials
    And a Cassandra cluster "production" is registered in AxonOps

  # --- Basic lifecycle ---

  Scenario: Create a basic log collector
    Given an AxonOpsLogCollector CR:
      | field         | value                                  |
      | connectionRef | test-connection                        |
      | clusterName   | production                             |
      | clusterType   | cassandra                              |
      | name          | GC Log Collector                       |
      | filename      | /var/log/cassandra/gc.log.0.current    |
    When the operator reconciles the AxonOpsLogCollector CR
    Then the AxonOps API should receive a GET to "/api/v1/logcollectors/{org}/cassandra/production"
    And the AxonOps API should receive a PUT to "/api/v1/logcollectors/{org}/cassandra/production"
    And the PUT payload should include a collector with "name" set to "GC Log Collector"
    And the PUT payload should include a collector with "filename" set to "/var/log/cassandra/gc.log.0.current"
    And the PUT payload collector should have a generated "uuid"
    And the CR status should have condition "Ready" with status "True"
    And the CR status should have a non-empty "syncedUUID"

  Scenario: Create a fully configured log collector
    Given an AxonOpsLogCollector CR:
      | field        | value                                  |
      | connectionRef| test-connection                        |
      | clusterName  | production                             |
      | clusterType  | cassandra                              |
      | name         | System Log                             |
      | filename     | /var/log/cassandra/system.log           |
      | interval     | 10s                                    |
      | timeout      | 2m                                     |
      | dateFormat   | yyyy-MM-ddTHH:mm:ssZ                   |
      | infoRegex    | INFO                                   |
      | warningRegex | WARN                                   |
      | errorRegex   | ERROR                                  |
      | debugRegex   | DEBUG                                  |
      | readonly     | true                                   |
    When the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload collector should have "interval" set to "10s"
    And the PUT payload collector should have "timeout" set to "2m"
    And the PUT payload collector should have "dateFormat" set to "yyyy-MM-ddTHH:mm:ssZ"
    And the PUT payload collector should have "infoRegex" set to "INFO"
    And the PUT payload collector should have "warningRegex" set to "WARN"
    And the PUT payload collector should have "errorRegex" set to "ERROR"
    And the PUT payload collector should have "debugRegex" set to "DEBUG"
    And the PUT payload collector should have "readonly" set to true

  Scenario: Default values applied when optional fields omitted
    Given an AxonOpsLogCollector CR with only required fields:
      | field         | value                                  |
      | connectionRef | test-connection                        |
      | clusterName   | production                             |
      | clusterType   | cassandra                              |
      | name          | Minimal Collector                      |
      | filename      | /var/log/cassandra/debug.log            |
    When the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload collector should have "interval" set to "5s"
    And the PUT payload collector should have "timeout" set to "1m"
    And the PUT payload collector should have "dateFormat" set to "yyyy-MM-dd HH:mm:ss,SSS"
    And the PUT payload collector should have "readonly" set to false

  # --- Collector identification by filename ---

  Scenario: Filename is the unique identifier for a log collector
    Given an existing log collector in AxonOps with filename "/var/log/cassandra/gc.log.0.current" and uuid "existing-uuid-123"
    And an AxonOpsLogCollector CR with filename "/var/log/cassandra/gc.log.0.current"
    And name "Updated GC Collector"
    When the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload should reuse uuid "existing-uuid-123" for the collector
    And the PUT payload collector should have "name" set to "Updated GC Collector"
    And the CR status "syncedUUID" should be "existing-uuid-123"

  Scenario: New filename creates a new collector with generated UUID
    Given no existing log collector in AxonOps with filename "/var/log/cassandra/new.log"
    And an AxonOpsLogCollector CR with filename "/var/log/cassandra/new.log"
    When the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload should include a collector with a newly generated UUID
    And the collector should not reuse any existing UUID

  # --- Update lifecycle ---

  Scenario: Update collector name triggers API sync
    Given an AxonOpsLogCollector CR with filename "/var/log/cassandra/gc.log" that is Ready
    When I update the name from "GC Log" to "GC Log v2"
    And the operator reconciles the AxonOpsLogCollector CR
    Then the AxonOps API should receive a PUT with the updated collector list
    And the collector with filename "/var/log/cassandra/gc.log" should have "name" set to "GC Log v2"
    And the CR status should have condition "Ready" with status "True"

  Scenario: Update collector regex triggers API sync
    Given an AxonOpsLogCollector CR with filename "/var/log/cassandra/system.log" that is Ready
    When I update errorRegex from "" to "ERROR|FATAL"
    And the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload collector should have "errorRegex" set to "ERROR|FATAL"

  # --- PUT payload structure ---

  Scenario: PUT payload preserves other collectors
    Given existing log collectors in AxonOps:
      | uuid   | filename                    | name         |
      | uuid-1 | /var/log/cassandra/gc.log   | GC Collector |
      | uuid-2 | /var/log/cassandra/sys.log  | Sys Collector|
    And an AxonOpsLogCollector CR with filename "/var/log/cassandra/gc.log"
    And name "Updated GC"
    When the operator reconciles the AxonOpsLogCollector CR
    Then the PUT payload should contain 2 collectors
    And one collector should have filename "/var/log/cassandra/gc.log" with name "Updated GC"
    And one collector should have filename "/var/log/cassandra/sys.log" with name "Sys Collector" unchanged

  Scenario: PUT payload includes mandatory integrations field
    Given an AxonOpsLogCollector CR with valid configuration
    When the operator reconciles the AxonOpsLogCollector CR
    Then each collector in the PUT payload should have an "integrations" field
    And the integrations field should have "OverrideError" set to false
    And the integrations field should have "OverrideInfo" set to false
    And the integrations field should have "OverrideWarning" set to false
    And the integrations field should have "Type" set to ""

  # --- Deletion ---

  Scenario: Delete log collector removes it from AxonOps
    Given an AxonOpsLogCollector CR with filename "/var/log/cassandra/gc.log" that is Ready
    And the collector has synced UUID "delete-uuid"
    And existing log collectors in AxonOps include this collector
    When I delete the AxonOpsLogCollector CR
    And the operator reconciles the deletion
    Then the AxonOps API should receive a PUT without the deleted collector
    And the PUT payload should not contain a collector with filename "/var/log/cassandra/gc.log"
    And the finalizer should be removed
    And the CR should no longer exist

  Scenario: Delete is idempotent when collector already removed from AxonOps
    Given an AxonOpsLogCollector CR with filename "/var/log/cassandra/removed.log" that is Ready
    And the AxonOps API returns no collector with that filename
    When I delete the AxonOpsLogCollector CR
    And the operator reconciles the deletion
    Then the finalizer should be removed
    And the CR should no longer exist

  # --- Cluster type support ---

  Scenario: Log collector for DSE cluster
    Given an AxonOpsLogCollector CR with clusterType "dse"
    And clusterName "dse-production"
    And filename "/var/log/dse/system.log"
    When the operator reconciles the AxonOpsLogCollector CR
    Then the AxonOps API should receive a GET to "/api/v1/logcollectors/{org}/dse/dse-production"
    And the AxonOps API should receive a PUT to "/api/v1/logcollectors/{org}/dse/dse-production"

  # --- Error handling ---

  Scenario: Connection not found sets Failed condition
    Given an AxonOpsLogCollector CR referencing connection "nonexistent"
    When the operator reconciles the AxonOpsLogCollector CR
    Then the CR status should have condition "Failed" with reason "FailedToResolveConnection"

  Scenario: API error on GET sets Failed condition with requeue
    Given an AxonOpsLogCollector CR with valid connection
    And the AxonOps API returns 500 Internal Server Error on GET logcollectors
    When the operator reconciles the AxonOpsLogCollector CR
    Then the CR status should have condition "Failed" with reason "SyncFailed"
    And the reconciliation should be requeued after 30 seconds

  Scenario: API error on PUT sets Failed condition with requeue
    Given an AxonOpsLogCollector CR with valid connection
    And the AxonOps API returns 200 on GET but 500 on PUT
    When the operator reconciles the AxonOpsLogCollector CR
    Then the CR status should have condition "Failed" with reason "SyncFailed"
    And the reconciliation should be requeued after 30 seconds

  Scenario: API returns 401 Unauthorized
    Given an AxonOpsLogCollector CR with valid connection
    And the AxonOps API returns 401 Unauthorized
    When the operator reconciles the AxonOpsLogCollector CR
    Then the CR status should have condition "Failed" with reason "AuthenticationFailed"

  # --- Idempotency ---

  Scenario: No API calls when collector is already synced and unchanged
    Given an AxonOpsLogCollector CR that is Ready with observedGeneration matching
    And syncedUUID is set
    When the operator reconciles the AxonOpsLogCollector CR
    Then no API calls should be made to AxonOps

  Scenario: Re-sync when observedGeneration does not match
    Given an AxonOpsLogCollector CR that is Ready
    But the spec has been modified (observedGeneration mismatch)
    When the operator reconciles the AxonOpsLogCollector CR
    Then the AxonOps API should receive a GET and PUT to sync the updated state

  # --- Validation ---

  Scenario: Filename is required
    Given an AxonOpsLogCollector CR without a filename
    When I attempt to apply the CR
    Then the CR should be rejected by validation with message containing "filename"

  Scenario: Name is required
    Given an AxonOpsLogCollector CR without a name
    When I attempt to apply the CR
    Then the CR should be rejected by validation with message containing "name"

  Scenario: ClusterType must be cassandra or dse
    Given an AxonOpsLogCollector CR with clusterType "invalid"
    When I attempt to apply the CR
    Then the CR should be rejected by validation with message containing "clusterType"
