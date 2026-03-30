Feature: OpenTelemetry instrumentation and Prometheus metrics
  As an operator user
  I want the AxonOps operator to export metrics and traces
  So that I can monitor operator health, performance, and resource lifecycle

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps operator is running and healthy
    And metrics collection is enabled (default)

  Scenario: Metrics endpoint exposes reconciliation metrics
    When a client requests metrics from the operator's metrics endpoint (http://localhost:8080/metrics)
    Then the response should be in Prometheus text format
    And the response should include the following metrics:
      | Metric Name                           | Type      | Labels                    |
      | axonops_reconciliation_duration_seconds | histogram | resource_type, result     |
      | axonops_reconciliation_total          | counter   | resource_type, result     |
      | axonops_resource_created_total        | counter   | resource_type             |
      | axonops_resource_deleted_total        | counter   | resource_type             |
      | axonops_resource_updated_total        | counter   | resource_type             |
      | axonops_reconciliation_errors_total   | counter   | error_type                |

  Scenario: Controller reconciliation duration is tracked accurately
    Given an AxonOpsPlatform CR is created
    When the controller reconciles the resource
    Then the axonops_reconciliation_duration_seconds metric should record:
      - A histogram bucket for the reconciliation duration
      - Resource type label (e.g., "axonopsplatform")
      - Result label (success or error)
      - Duration in seconds
    And the axonops_reconciliation_total counter should be incremented
      - With the same resource type and result labels

  Scenario: Resource lifecycle metrics track created resources
    Given the AxonOpsPlatform controller is reconciling resources
    When a StatefulSet is created for TimeSeries
    Then axonops_resource_created_total should be incremented with:
      - resource_type label = "statefulset"
      - Subsequent metrics creation for other resources (service, secret, configmap)
    And when other resources are created
    Then each resource creation should increment the metric with appropriate type label

  Scenario: Resource lifecycle metrics track deleted resources
    Given an AxonOpsPlatform with internal resources deployed
    When the controller cleanup deletes a StatefulSet
    Then axonops_resource_deleted_total should be incremented with:
      - resource_type label = "statefulset"
      - Subsequent metrics for service, secret deletion

  Scenario: Error metrics track failure types
    Given the AxonOpsPlatform controller encounters errors
    When reconciliation fails (e.g., API error, validation error, conflict)
    Then axonops_reconciliation_errors_total should be incremented with:
      - error_type label indicating the failure type
      - Result label = "error" in reconciliation_total metric

  Scenario: ServiceMonitor is created for Prometheus integration
    Given the Prometheus Operator is installed in the cluster
    When the AxonOps operator is deployed
    Then a ServiceMonitor resource should be created that:
      - Targets the operator's metrics service
      - Scrapes metrics every 30 seconds
      - Selects the operator's service by labels
      - Is labeled with operator=axonops-operator

  Scenario: OpenTelemetry traces are exported to configured endpoint
    Given the AxonOps operator is configured with OTEL_EXPORTER_OTLP_ENDPOINT
    And an OTLP collector is listening on that endpoint
    When a reconciliation event occurs
    Then a trace should be exported with:
      - Operation name: reconcile.<resource-type>
      - Spans for: fetch CR, reconcile components, update status
      - Attributes: resource name, namespace, result, duration
      - Proper parent-child relationship between spans

  Scenario: Trace spans record component reconciliation
    Given the AxonOpsPlatform controller is reconciling
    When the controller processes components (TimeSeries, Search, Server, Dashboard)
    Then each component should have a span with:
      - Span name: reconcile.<component-name>
      - Attributes: component status, duration, result
      - Links to parent reconciliation span
      - Error status if component reconciliation failed

  Scenario: Metrics can be disabled for performance
    Given the AxonOps operator is running
    When DISABLE_METRICS environment variable is set to "true"
    Then:
      - The metrics endpoint should not be exposed
      - No OTLP exporter connections should be made
      - No metric recording overhead should be incurred
      - Operator should log that metrics collection is disabled

  Scenario: Metrics include proper label dimensions
    Given the AxonOpsPlatform controller is running
    When multiple AxonOpsPlatform resources are reconciled in different namespaces
    Then metrics should include appropriate labels for:
      - resource_type (axonopsplatform, axonopsmetricalert, etc.)
      - result (success, error)
      - error_type (for errors: api_error, validation_error, timeout, conflict, other)
      - namespace (optional, for filtered views)

  Scenario: Reconciliation metrics reflect actual performance
    Given an AxonOpsPlatform with all components configured
    When the controller completes reconciliation
    Then the axonops_reconciliation_duration_seconds histogram should show:
      - Reasonable duration (seconds)
      - Appropriate histogram buckets (100ms, 500ms, 1s, 5s, 10s, 30s, 60s)
      - P95 and P99 latencies available via histogram quantiles

  Scenario: Multiple reconciliation attempts are tracked
    Given an AxonOpsPlatform CR is created
    When the controller reconciles it multiple times (including retries)
    Then axonops_reconciliation_total should accumulate:
      - Each reconciliation attempt counted
      - Success or error result for each attempt
      - Running total of all attempts

  Scenario: Prometheus dashboard visualizes operator health
    Given metrics are being exported from the operator
    When a Grafana dashboard is imported using provided JSON
    Then the dashboard should display:
      - Reconciliation success rate (%)
      - Reconciliation latency (P50/P95/P99)
      - Error rate and error types
      - Resource creation/deletion rate
      - Controller availability/uptime
      - Metric values updated in real-time
