Feature: Configuration changes trigger pod rolling updates
  As a cluster operator
  I want pods to automatically restart when their configuration changes
  So that running workloads always reflect the current desired state

  Background:
    Given a Kubernetes cluster is available
    And the AxonOps operator is running
    And the CRDs are installed

  # ── Server component ──────────────────────────────────────────────

  Scenario: Server config Secret change triggers StatefulSet rolling update
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the server StatefulSet pods are running
    And I record the server StatefulSet pod template "checksum/config" annotation value
    When I update the AxonOpsPlatform CR "test-server" spec.server.orgName to "new-org"
    Then the server config Secret "test-server-server" data changes
    And the server StatefulSet pod template "checksum/config" annotation has a different value
    And the server StatefulSet performs a rolling update
    And eventually all server pods are replaced with new pods running the updated config

  Scenario: Server config Secret unchanged when irrelevant field changes
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the server StatefulSet pods are running
    And I record the server StatefulSet pod template "checksum/config" annotation value
    When I update the AxonOpsPlatform CR "test-server" spec.server.replicas to 2
    Then the server StatefulSet pod template "checksum/config" annotation has the same value

  # ── Dashboard component ───────────────────────────────────────────

  Scenario: Dashboard ConfigMap change triggers Deployment rolling update
    Given an AxonOpsPlatform CR "test-server" is deployed with dashboard component enabled
    And the dashboard Deployment pods are running
    And I record the dashboard Deployment pod template "checksum/config" annotation value
    When I update the AxonOpsPlatform CR "test-server" spec.dashboard configuration
    Then the dashboard ConfigMap "test-server-dash" data changes
    And the dashboard Deployment pod template "checksum/config" annotation has a different value
    And the dashboard Deployment performs a rolling update
    And eventually all dashboard pods are replaced with new pods running the updated config

  # ── TimeSeries component ──────────────────────────────────────────

  Scenario: TimeSeries auth Secret change triggers StatefulSet rolling update
    Given an AxonOpsPlatform CR "test-server" is deployed with internal timeseries enabled
    And the timeseries StatefulSet pods are running
    And I record the timeseries StatefulSet pod template "checksum/auth" annotation value
    When the timeseries auth Secret "test-server-timeseries-auth" data is updated with new credentials
    Then the timeseries StatefulSet pod template "checksum/auth" annotation has a different value
    And the timeseries StatefulSet performs a rolling update
    And eventually all timeseries pods are replaced with new pods

  Scenario: TimeSeries auth Secret change via CR password update
    Given an AxonOpsPlatform CR "test-server" is deployed with internal timeseries enabled
    And the timeseries StatefulSet pods are running
    And I record the timeseries StatefulSet pod template "checksum/auth" annotation value
    When I update the AxonOpsPlatform CR "test-server" spec.timeSeries.authentication.password
    Then the timeseries auth Secret data changes
    And the timeseries StatefulSet pod template "checksum/auth" annotation has a different value
    And the timeseries StatefulSet performs a rolling update

  # ── Search component ──────────────────────────────────────────────

  Scenario: Search auth Secret change triggers StatefulSet rolling update
    Given an AxonOpsPlatform CR "test-server" is deployed with internal search enabled
    And the search StatefulSet pods are running
    And I record the search StatefulSet pod template "checksum/auth" annotation value
    When the search auth Secret "test-server-search-auth" data is updated with new credentials
    Then the search StatefulSet pod template "checksum/auth" annotation has a different value
    And the search StatefulSet performs a rolling update
    And eventually all search pods are replaced with new pods

  # ── Annotation merging ────────────────────────────────────────────

  Scenario: User-provided annotations are preserved alongside checksum annotations
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the CR spec.server.annotations includes "custom-key: custom-value"
    When the operator reconciles the server StatefulSet
    Then the server StatefulSet pod template annotations include "custom-key" with value "custom-value"
    And the server StatefulSet pod template annotations include "checksum/config"

  Scenario: Controller-computed checksum overwrites user-set checksum annotation
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the CR spec.server.annotations includes "checksum/config: user-provided-value"
    When the operator reconciles the server StatefulSet
    Then the server StatefulSet pod template "checksum/config" annotation is NOT "user-provided-value"
    And the server StatefulSet pod template "checksum/config" annotation matches the SHA-256 hash of the config Secret data

  # ── Error handling ────────────────────────────────────────────────

  Scenario: Missing config Secret prevents StatefulSet creation
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the server config Secret "test-server-server" does not exist
    When the operator attempts to reconcile the server StatefulSet
    Then the reconciliation returns an error indicating the config Secret is missing
    And no server StatefulSet is created without a checksum annotation

  # ── Hash determinism ──────────────────────────────────────────────

  Scenario: Identical config produces identical checksum across reconciliations
    Given an AxonOpsPlatform CR "test-server" is deployed with server component enabled
    And the server StatefulSet pods are running
    And I record the server StatefulSet pod template "checksum/config" annotation value
    When the operator reconciles the server StatefulSet again without any spec changes
    Then the server StatefulSet pod template "checksum/config" annotation has the same value
    And no rolling update is triggered
