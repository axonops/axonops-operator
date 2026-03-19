Feature: AxonOpsDashboardTemplate lifecycle for declarative dashboard management
  As an operator user
  I want to declare dashboard templates using AxonOpsDashboardTemplate CRs
  So that dashboard configuration is managed declaratively via GitOps

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR "my-connection" exists in namespace "default"

  # --- Inline source ---

  Scenario: Create dashboard from inline source with default settings
    Given an AxonOpsDashboardTemplate CR with:
      | field          | value          |
      | connectionRef  | my-connection  |
      | clusterName    | my-cluster     |
      | clusterType    | cassandra      |
      | dashboardName  | CPU Overview   |
    And the source is inline with a dashboard containing 2 panels and 1 filter
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the AxonOps API should receive a GET to /api/v1/dashboardtemplate with dashver=2.0
    And the AxonOps API should receive a PUT to /api/v1/dashboardtemplate with dashver=2.0
    And the PUT payload should contain a dashboard named "CPU Overview" with 2 panels and 1 filter
    And the status condition "Ready" should be "True"
    And the status panelCount should be 2
    And the status should contain a lastSyncTime
    And the status observedGeneration should match the CR generation

  Scenario: Create dashboard from inline source for DSE cluster
    Given an AxonOpsDashboardTemplate CR with clusterType="dse"
    And the source is inline with a dashboard containing 5 panels
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the PUT payload type field should be "dse"
    And the status condition "Ready" should be "True"

  Scenario: Update inline dashboard panels
    Given an existing AxonOpsDashboardTemplate CR "CPU Overview" with Ready=True and 2 panels
    When the user updates the inline source to contain 4 panels
    And the operator reconciles the AxonOpsDashboardTemplate CR
    Then the AxonOps API should receive a PUT containing "CPU Overview" with 4 panels
    And the status panelCount should be 4
    And the status condition "Ready" should be "True"

  Scenario: No API call when inline spec has not changed
    Given an existing AxonOpsDashboardTemplate CR with Ready=True
    And the status observedGeneration matches the CR generation
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then no API call should be made to the AxonOps dashboard template endpoint
    And the status should remain unchanged

  # --- ConfigMap source ---

  Scenario: Create dashboard from ConfigMap source
    Given a ConfigMap "cpu-dashboard" in namespace "default" with key "dashboard.json" containing:
      """
      {
        "filters": [
          {"name": "dc", "label": "data center", "type": "query", "multi": true, "query": "host_CPU_Percent_Merge", "regex": "dc=([^:]+)"}
        ],
        "panels": [
          {"title": "CPU Usage", "type": "line-chart", "uuid": "panel-1", "layout": {"h": 3, "w": 9, "x": 0, "y": 0}, "details": {"queries": [{"query": "host_CPU_Percent_Merge{dc=~'$dc'}"}]}}
        ]
      }
      """
    And an AxonOpsDashboardTemplate CR referencing ConfigMap "cpu-dashboard" key "dashboard.json"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the AxonOps API should receive a PUT with dashboard "CPU Overview" containing 1 panel
    And the status condition "Ready" should be "True"
    And the status panelCount should be 1

  Scenario: ConfigMap change triggers re-reconciliation
    Given an existing AxonOpsDashboardTemplate CR referencing ConfigMap "cpu-dashboard"
    And the CR is synced with Ready=True and panelCount=1
    When the user updates ConfigMap "cpu-dashboard" to contain 3 panels
    Then the operator should re-reconcile the AxonOpsDashboardTemplate CR
    And the AxonOps API should receive a PUT with the updated 3 panels
    And the status panelCount should be 3

  Scenario: ConfigMap with large dashboard (approaching size limits)
    Given a ConfigMap "large-dashboard" containing a 400KB dashboard JSON
    And an AxonOpsDashboardTemplate CR referencing ConfigMap "large-dashboard"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then a warning should be logged about large dashboard size
    And the status condition "Ready" should be "True"

  # --- Merge behaviour ---

  Scenario: New dashboard appended to existing dashboards
    Given the AxonOps API returns 2 existing dashboards: "Disk I/O" and "Network"
    And an AxonOpsDashboardTemplate CR for dashboard "CPU Overview"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the PUT payload should contain 3 dashboards: "Disk I/O", "Network", and "CPU Overview"
    And dashboards "Disk I/O" and "Network" should be unchanged from the GET response

  Scenario: Existing dashboard replaced by name
    Given the AxonOps API returns 3 dashboards including "CPU Overview" with 2 panels
    And an AxonOpsDashboardTemplate CR for "CPU Overview" with 5 panels
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the PUT payload should contain 3 dashboards
    And "CPU Overview" in the PUT payload should have 5 panels
    And the other 2 dashboards should be unchanged

  # --- Deletion ---

  Scenario: Delete dashboard CR removes dashboard from remote list
    Given an existing AxonOpsDashboardTemplate CR "CPU Overview" with Ready=True
    And the AxonOps API returns 3 dashboards including "CPU Overview"
    When the user deletes the AxonOpsDashboardTemplate CR
    And the operator reconciles the deletion
    Then the AxonOps API should receive a PUT with 2 dashboards (without "CPU Overview")
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Delete when API unreachable retries without removing finalizer
    Given an existing AxonOpsDashboardTemplate CR "CPU Overview" with Ready=True
    And the AxonOps API is unreachable
    When the user deletes the AxonOpsDashboardTemplate CR
    And the operator reconciles the deletion
    Then the finalizer should NOT be removed
    And the CR should be requeued for retry after 30 seconds

  Scenario: Delete when dashboard not found in remote list
    Given an existing AxonOpsDashboardTemplate CR "CPU Overview" with Ready=True
    And the AxonOps API returns 2 dashboards that do NOT include "CPU Overview"
    When the user deletes the AxonOpsDashboardTemplate CR
    And the operator reconciles the deletion
    Then no PUT should be sent (list is already correct)
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  # --- Validation errors ---

  Scenario: Invalid source - neither inline nor configMapRef set
    Given an AxonOpsDashboardTemplate CR with empty source
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "InvalidSource"
    And the CR should NOT be requeued

  Scenario: Invalid source - both inline and configMapRef set
    Given an AxonOpsDashboardTemplate CR with both inline and configMapRef source
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "InvalidSource"
    And the CR should NOT be requeued

  Scenario: ConfigMap not found
    Given an AxonOpsDashboardTemplate CR referencing ConfigMap "nonexistent"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "ConfigMapNotFound"
    And the CR should be requeued for retry after 30 seconds

  Scenario: ConfigMap key missing
    Given a ConfigMap "my-cm" without key "dashboard.json"
    And an AxonOpsDashboardTemplate CR referencing ConfigMap "my-cm" key "dashboard.json"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "ConfigMapKeyNotFound"
    And the CR should NOT be requeued

  Scenario: ConfigMap value is not valid JSON
    Given a ConfigMap "bad-json" with key "dashboard.json" containing "not valid json {{"
    And an AxonOpsDashboardTemplate CR referencing ConfigMap "bad-json" key "dashboard.json"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "InvalidDashboardJSON"
    And the CR should NOT be requeued

  # --- API errors ---

  Scenario: AxonOps API returns server error on GET
    Given an AxonOpsDashboardTemplate CR with a valid connection and inline source
    And the AxonOps API GET returns HTTP 500
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should be requeued for retry after 30 seconds

  Scenario: AxonOps API returns client error on PUT
    Given an AxonOpsDashboardTemplate CR with a valid connection and inline source
    And the AxonOps API PUT returns HTTP 400
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should NOT be requeued

  Scenario: AxonOps API returns server error on PUT
    Given an AxonOpsDashboardTemplate CR with a valid connection and inline source
    And the AxonOps API PUT returns HTTP 503
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should be requeued for retry after 30 seconds

  # --- Connection errors ---

  Scenario: Failed condition when AxonOpsConnection reference is invalid
    Given an AxonOpsDashboardTemplate CR with connectionRef="nonexistent"
    When the operator reconciles the AxonOpsDashboardTemplate CR
    Then the status condition "Failed" should be "True" with reason "ConnectionError"
    And the CR should be requeued for retry after 30 seconds
