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
	"context"
	"fmt"
	"os"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// panelInfo maps a panel UUID to its dashboard name and chart title.
type panelInfo struct {
	Dashboard string
	Chart     string
}

// exportMetricAlerts fetches metric alert rules from the AxonOps API and
// converts them to AxonOpsMetricAlert CRD resources.
// Log alert rules (expr starts with "events{") are excluded.
func exportMetricAlerts(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	rules, err := client.GetMetricAlertRules(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching metric alert rules: %w", err)
	}

	// Build a reverse lookup: correlationId → (dashboard, chart)
	panelIndex, err := buildPanelIndex(ctx, client, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  warning: could not load dashboard templates for panel resolution: %v\n", err)
		// Continue without dashboard resolution — chart comes from WidgetTitle
	}

	var resources []Resource
	for _, rule := range rules {
		if isLogAlertExpr(rule.Expr) {
			continue // log alerts handled separately
		}
		r := metricAlertRuleToResource(rule, opts, panelIndex)
		resources = append(resources, r)
	}
	return resources, nil
}

// exportLogAlerts fetches alert rules whose expression matches the log events
// format ("events{...}") and converts them to AxonOpsLogAlert CRD resources.
func exportLogAlerts(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	rules, err := client.GetMetricAlertRules(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching log alert rules: %w", err)
	}

	var resources []Resource
	for _, rule := range rules {
		if !isLogAlertExpr(rule.Expr) {
			continue
		}
		r := logAlertRuleToResource(rule, opts)
		resources = append(resources, r)
	}
	return resources, nil
}

// isLogAlertExpr returns true when the expression belongs to a log alert.
// Log alert expressions start with "events{".
func isLogAlertExpr(expr string) bool {
	return strings.HasPrefix(strings.TrimSpace(expr), "events{")
}

// logAlertRuleToResource converts a log-alert MetricAlertRule to an AxonOpsLogAlert resource.
func logAlertRuleToResource(rule axonops.MetricAlertRule, opts *Options) Resource {
	name := toDNSLabel(rule.Alert)

	alert := &alertsv1alpha1.AxonOpsLogAlert{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsLogAlert",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsLogAlertSpec{
			ConnectionRef: opts.ConnectionName,
			ClusterName:   opts.ClusterName,
			ClusterType:   opts.ClusterType,
			Name:          rule.Alert,
			Operator:      rule.Operator,
			WarningValue:  rule.WarningValue,
			CriticalValue: rule.CriticalValue,
			Duration:      rule.For,
		},
	}

	// Parse expr "events{message="x",level="y",source="z",type="t"}"
	content, level, source, logType := parseLogEventsExpr(rule.Expr)
	alert.Spec.Content = content
	alert.Spec.Level = level
	alert.Spec.Source = source
	alert.Spec.LogType = logType

	if rule.Annotations.Summary != "" || rule.Annotations.Description != "" {
		alert.Spec.Annotations = &alertsv1alpha1.LogAlertAnnotations{
			Summary:     rule.Annotations.Summary,
			Description: rule.Annotations.Description,
		}
	}

	// Filters
	if len(rule.Filters) > 0 {
		f := &alertsv1alpha1.LogAlertFilters{}
		for _, af := range rule.Filters {
			switch af.Name {
			case "dc":
				f.DataCenter = af.Value
			case "rack":
				f.Rack = af.Value
			case "host_id":
				f.HostID = af.Value
			}
		}
		alert.Spec.Filters = f
	}

	return Resource{Kind: "AxonOpsLogAlert", Name: name, Object: alert}
}

// parseLogEventsExpr parses content, level, source, logType from an events expression.
// Format: events{message="x",level="y",source="z",type="t"}
func parseLogEventsExpr(expr string) (content, level, source, logType string) {
	// Strip outer "events{...}"
	inner := expr
	if i := strings.Index(inner, "{"); i >= 0 {
		inner = inner[i+1:]
	}
	if i := strings.LastIndex(inner, "}"); i >= 0 {
		inner = inner[:i]
	}

	// Simple key="value" parser (handles escaped quotes inside values)
	for len(inner) > 0 {
		eqIdx := strings.Index(inner, "=")
		if eqIdx < 0 {
			break
		}
		key := strings.TrimSpace(inner[:eqIdx])
		rest := inner[eqIdx+1:]
		if len(rest) == 0 || rest[0] != '"' {
			break
		}
		rest = rest[1:] // skip opening quote
		// Find closing unescaped quote
		var valBuf strings.Builder
		i := 0
		for i < len(rest) {
			if rest[i] == '\\' && i+1 < len(rest) {
				valBuf.WriteByte(rest[i+1])
				i += 2
				continue
			}
			if rest[i] == '"' {
				i++
				break
			}
			valBuf.WriteByte(rest[i])
			i++
		}
		val := valBuf.String()
		switch key {
		case "message":
			content = val
		case "level":
			level = strings.ReplaceAll(val, "|", ",")
		case "source":
			source = strings.ReplaceAll(val, "|", ",")
		case "type":
			logType = strings.ReplaceAll(val, "|", ",")
		}
		inner = strings.TrimLeft(rest[i:], ",")
	}
	return content, level, source, logType
}

// buildPanelIndex fetches all dashboards and builds a map from panel UUID to dashboard+chart info.
func buildPanelIndex(ctx context.Context, client *axonops.Client, opts *Options) (map[string]panelInfo, error) {
	dashboards, err := client.GetDashboards(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, err
	}

	index := make(map[string]panelInfo)
	for _, dash := range dashboards {
		for _, panel := range dash.Panels {
			index[panel.UUID] = panelInfo{
				Dashboard: dash.Name,
				Chart:     panel.Title,
			}
		}
	}
	return index, nil
}

// metricAlertRuleToResource converts a single API MetricAlertRule to a
// Kubernetes AxonOpsMetricAlert resource.
func metricAlertRuleToResource(rule axonops.MetricAlertRule, opts *Options, panelIndex map[string]panelInfo) Resource {
	name := toDNSLabel(rule.Alert)

	// Extract metric name from the expression.
	// Expression format: "<metric> <operator> <value>"
	metric := extractMetric(rule.Expr)

	alert := &alertsv1alpha1.AxonOpsMetricAlert{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsMetricAlert",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsMetricAlertSpec{
			ConnectionRef: opts.ConnectionName,
			ClusterName:   opts.ClusterName,
			ClusterType:   opts.ClusterType,
			Name:          rule.Alert,
			Operator:      rule.Operator,
			WarningValue:  rule.WarningValue,
			CriticalValue: rule.CriticalValue,
			Duration:      rule.For,
			Metric:        metric,
		},
	}

	// Resolve dashboard and chart.
	// First try the panel index (correlationId → dashboard name + chart title).
	// Fall back to WidgetTitle from the alert rule for the chart name.
	if pi, ok := panelIndex[rule.CorrelationId]; ok {
		alert.Spec.Dashboard = pi.Dashboard
		alert.Spec.Chart = pi.Chart
	} else if rule.WidgetTitle != "" {
		alert.Spec.Chart = rule.WidgetTitle
		alert.Spec.Dashboard = "<UNKNOWN_DASHBOARD>"
	}

	// Annotations
	if rule.Annotations.Summary != "" || rule.Annotations.Description != "" {
		alert.Spec.Annotations = &alertsv1alpha1.MetricAlertAnnotations{
			Summary:     rule.Annotations.Summary,
			Description: rule.Annotations.Description,
			WidgetURL:   rule.Annotations.WidgetUrl,
		}
	}

	// Filters
	if filters := convertFilters(rule.Filters); filters != nil {
		alert.Spec.Filters = filters
	}

	return Resource{
		Kind:   "AxonOpsMetricAlert",
		Name:   name,
		Object: alert,
	}
}

// extractMetric parses the metric name from an expression like "metric_name >= 50".
func extractMetric(expr string) string {
	parts := strings.Fields(expr)
	if len(parts) >= 1 {
		return parts[0]
	}
	return expr
}

// convertFilters converts API MetricAlertFilter list to CRD MetricAlertFilters.
func convertFilters(apiFilters []axonops.MetricAlertFilter) *alertsv1alpha1.MetricAlertFilters {
	if len(apiFilters) == 0 {
		return nil
	}

	f := &alertsv1alpha1.MetricAlertFilters{}
	for _, af := range apiFilters {
		if len(af.Value) == 0 {
			continue
		}
		switch af.Name {
		case "dc":
			f.DataCenter = af.Value
		case "rack":
			f.Rack = af.Value
		case "host_id":
			f.HostID = af.Value
		case "scope":
			f.Scope = af.Value
		case "keyspace":
			f.Keyspace = af.Value
		case "percentile":
			f.Percentile = af.Value
		case "consistency":
			f.Consistency = af.Value
		case "topic":
			f.Topic = af.Value
		case "group_id":
			f.GroupID = af.Value
		case "group_by":
			f.GroupBy = af.Value
		}
	}
	return f
}
