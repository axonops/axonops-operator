/*
© 2026 AxonOps Limited. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package export

import (
	"testing"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

func TestExtractMetric(t *testing.T) {
	tests := []struct {
		expr     string
		expected string
	}{
		{"cassandra_read_latency_ms >= 50", "cassandra_read_latency_ms"},
		{"cpu_usage > 80", "cpu_usage"},
		{"metric", "metric"},
		{"", ""},
		{"some_metric != 0", "some_metric"},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			got := extractMetric(tt.expr)
			if got != tt.expected {
				t.Errorf("extractMetric(%q) = %q, want %q", tt.expr, got, tt.expected)
			}
		})
	}
}

func TestConvertFilters(t *testing.T) {
	// Empty filters
	if got := convertFilters(nil); got != nil {
		t.Errorf("convertFilters(nil) = %v, want nil", got)
	}
	if got := convertFilters([]axonops.MetricAlertFilter{}); got != nil {
		t.Errorf("convertFilters([]) = %v, want nil", got)
	}

	// Populated filters
	apiFilters := []axonops.MetricAlertFilter{
		{Name: "dc", Value: []string{"dc1", "dc2"}},
		{Name: "host_id", Value: []string{"host-1"}},
		{Name: "keyspace", Value: []string{"system"}},
		{Name: "group_by", Value: []string{"host_id"}},
		{Name: "unknown_filter", Value: []string{"val"}}, // Should be ignored
		{Name: "rack", Value: []string{}},                // Empty value, should be skipped
	}

	got := convertFilters(apiFilters)
	if got == nil {
		t.Fatal("convertFilters returned nil for non-empty input")
	}

	assertStrSlice(t, "DataCenter", got.DataCenter, []string{"dc1", "dc2"})
	assertStrSlice(t, "HostID", got.HostID, []string{"host-1"})
	assertStrSlice(t, "Keyspace", got.Keyspace, []string{"system"})
	assertStrSlice(t, "GroupBy", got.GroupBy, []string{"host_id"})
	assertStrSlice(t, "Rack", got.Rack, nil) // Empty value was skipped
}

func TestMetricAlertRuleToResource(t *testing.T) {
	opts := &Options{
		ClusterName:    "prod-cluster",
		ClusterType:    "cassandra",
		Namespace:      "monitoring",
		ConnectionName: "my-conn",
	}

	panelIndex := map[string]panelInfo{
		"panel-uuid-123": {Dashboard: "System", Chart: "CPU Usage"},
	}

	rule := axonops.MetricAlertRule{
		ID:            "alert-1",
		Alert:         "High CPU Usage",
		For:           "15m",
		Operator:      ">=",
		WarningValue:  80,
		CriticalValue: 95,
		Expr:          "cpu_usage >= 80",
		WidgetTitle:   "CPU Usage",
		CorrelationId: "panel-uuid-123",
		Annotations: axonops.MetricAlertAnnotations{
			Summary:     "CPU is high",
			Description: "CPU usage exceeded threshold",
		},
		Filters: []axonops.MetricAlertFilter{
			{Name: "dc", Value: []string{"dc1"}},
		},
	}

	r := metricAlertRuleToResource(rule, opts, panelIndex)

	if r.Kind != "AxonOpsMetricAlert" {
		t.Errorf("Kind = %q, want AxonOpsMetricAlert", r.Kind)
	}
	if r.Name != "high-cpu-usage" {
		t.Errorf("Name = %q, want high-cpu-usage", r.Name)
	}

	alert := r.Object.(*alertsv1alpha1.AxonOpsMetricAlert)

	if alert.Spec.ConnectionRef != "my-conn" {
		t.Errorf("ConnectionRef = %q, want my-conn", alert.Spec.ConnectionRef)
	}
	if alert.Spec.Name != "High CPU Usage" {
		t.Errorf("Name = %q, want 'High CPU Usage'", alert.Spec.Name)
	}
	if alert.Spec.Operator != ">=" {
		t.Errorf("Operator = %q, want >=", alert.Spec.Operator)
	}
	if alert.Spec.WarningValue != 80 {
		t.Errorf("WarningValue = %f, want 80", alert.Spec.WarningValue)
	}
	if alert.Spec.CriticalValue != 95 {
		t.Errorf("CriticalValue = %f, want 95", alert.Spec.CriticalValue)
	}
	if alert.Spec.Duration != "15m" {
		t.Errorf("Duration = %q, want 15m", alert.Spec.Duration)
	}
	if alert.Spec.Metric != "cpu_usage" {
		t.Errorf("Metric = %q, want cpu_usage", alert.Spec.Metric)
	}
	// Dashboard resolved from panel index
	if alert.Spec.Dashboard != "System" {
		t.Errorf("Dashboard = %q, want System", alert.Spec.Dashboard)
	}
	if alert.Spec.Chart != "CPU Usage" {
		t.Errorf("Chart = %q, want CPU Usage", alert.Spec.Chart)
	}
	// Annotations
	if alert.Spec.Annotations == nil {
		t.Fatal("Annotations is nil")
	}
	if alert.Spec.Annotations.Summary != "CPU is high" {
		t.Errorf("Summary = %q, want 'CPU is high'", alert.Spec.Annotations.Summary)
	}
	// Filters
	if alert.Spec.Filters == nil {
		t.Fatal("Filters is nil")
	}
	assertStrSlice(t, "Filters.DataCenter", alert.Spec.Filters.DataCenter, []string{"dc1"})
	// Namespace
	if alert.Namespace != "monitoring" {
		t.Errorf("Namespace = %q, want monitoring", alert.Namespace)
	}
}

func TestMetricAlertRuleToResource_NoPanelIndex(t *testing.T) {
	opts := &Options{
		ClusterName:    "test",
		ClusterType:    "cassandra",
		Namespace:      "default",
		ConnectionName: "conn",
	}

	rule := axonops.MetricAlertRule{
		Alert:       "test-alert",
		Expr:        "metric > 10",
		Operator:    ">",
		WidgetTitle: "Some Chart",
	}

	r := metricAlertRuleToResource(rule, opts, nil)
	alert := r.Object.(*alertsv1alpha1.AxonOpsMetricAlert)

	// Without panel index, falls back to WidgetTitle
	if alert.Spec.Chart != "Some Chart" {
		t.Errorf("Chart = %q, want 'Some Chart'", alert.Spec.Chart)
	}
	if alert.Spec.Dashboard != "<UNKNOWN_DASHBOARD>" {
		t.Errorf("Dashboard = %q, want <UNKNOWN_DASHBOARD>", alert.Spec.Dashboard)
	}
}

func assertStrSlice(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len = %d, want %d (%v vs %v)", name, len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", name, i, got[i], want[i])
		}
	}
}
