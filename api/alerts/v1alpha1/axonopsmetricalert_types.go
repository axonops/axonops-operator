/*
Copyright 2026.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AxonOpsMetricAlertSpec defines the desired state of AxonOpsMetricAlert
type AxonOpsMetricAlertSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// If not provided, falls back to operator environment variables.
	// +optional
	ConnectionRef string `json:"connectionRef,omitempty"`

	// ClusterName is the name of the cluster this alert applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, kafka, dse)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;kafka;dse
	ClusterType string `json:"clusterType"`

	// Name is the human-readable name of the alert rule
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Operator is the comparison operator (>, >=, =, !=, <=, <)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=>;>=;=;!=;<=;<
	Operator string `json:"operator"`

	// WarningValue is the warning threshold
	// +kubebuilder:validation:Required
	WarningValue float64 `json:"warningValue"`

	// CriticalValue is the critical threshold
	// +kubebuilder:validation:Required
	CriticalValue float64 `json:"criticalValue"`

	// Duration is how long the condition must be true to trigger the alert (e.g. 15m, 1h)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Duration string `json:"duration"`

	// Dashboard is the name of the dashboard containing the chart
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Dashboard string `json:"dashboard"`

	// Chart is the name of the chart/panel within the dashboard
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Chart string `json:"chart"`

	// Metric is the base metric name for the alert expression (e.g., "cassandra_read_latency_ms")
	// This is required and must match a metric from the dashboard chart query
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Metric string `json:"metric"`

	// Annotations contains optional alert metadata
	// +optional
	Annotations *MetricAlertAnnotations `json:"annotations,omitempty"`

	// Integrations specifies how to route alert notifications
	// +optional
	// +listType=atomic
	Integrations []MetricAlertIntegration `json:"integrations,omitempty"`

	// Filters apply constraints to the alert based on metric dimensions
	// +optional
	Filters *MetricAlertFilters `json:"filters,omitempty"`
}

// MetricAlertAnnotations defines optional alert metadata
type MetricAlertAnnotations struct {
	// Summary is a short summary of the alert
	// +optional
	Summary string `json:"summary,omitempty"`

	// Description is a longer description of the alert
	// +optional
	Description string `json:"description,omitempty"`

	// WidgetURL is a URL to the dashboard widget
	// +optional
	WidgetURL string `json:"widgetUrl,omitempty"`
}

// MetricAlertIntegration defines alert notification routing
type MetricAlertIntegration struct {
	// Type is the integration type (e.g., email, slack, pagerduty)
	// +optional
	Type string `json:"type,omitempty"`

	// Routing is the list of routing destinations (email addresses, Slack channels, etc.)
	// +optional
	// +listType=atomic
	Routing []string `json:"routing,omitempty"`

	// OverrideInfo determines if the integration overrides info-level alerts
	// +optional
	OverrideInfo bool `json:"overrideInfo,omitempty"`

	// OverrideWarning determines if the integration overrides warning-level alerts
	// +optional
	OverrideWarning bool `json:"overrideWarning,omitempty"`

	// OverrideError determines if the integration overrides error-level alerts
	// +optional
	OverrideError bool `json:"overrideError,omitempty"`
}

// MetricAlertFilters defines optional constraints for alert matching
type MetricAlertFilters struct {
	// DataCenter filters alerts to specific data centers
	// +optional
	// +listType=atomic
	DataCenter []string `json:"dc,omitempty"`

	// Rack filters alerts to specific racks
	// +optional
	// +listType=atomic
	Rack []string `json:"rack,omitempty"`

	// HostID filters alerts to specific hosts
	// +optional
	// +listType=atomic
	HostID []string `json:"hostId,omitempty"`

	// Scope filters alerts by scope (e.g., cluster, keyspace)
	// +optional
	// +listType=atomic
	Scope []string `json:"scope,omitempty"`

	// Keyspace filters alerts to specific Cassandra keyspaces
	// +optional
	// +listType=atomic
	Keyspace []string `json:"keyspace,omitempty"`

	// Percentile filters alerts to specific percentiles (e.g., 75thPercentile, 95thPercentile)
	// +optional
	// +listType=atomic
	Percentile []string `json:"percentile,omitempty"`

	// Consistency filters alerts to specific Cassandra consistency levels
	// +optional
	// +listType=atomic
	Consistency []string `json:"consistency,omitempty"`

	// Topic filters alerts to specific Kafka topics
	// +optional
	// +listType=atomic
	Topic []string `json:"topic,omitempty"`

	// GroupID filters alerts to specific Kafka consumer groups
	// +optional
	// +listType=atomic
	GroupID []string `json:"groupId,omitempty"`

	// GroupBy specifies dimensions to group alert instances by
	// +optional
	// +listType=atomic
	GroupBy []string `json:"groupBy,omitempty"`
}

// AxonOpsMetricAlertStatus defines the observed state of AxonOpsMetricAlert.
type AxonOpsMetricAlertStatus struct {
	// Conditions represent the current state of the AxonOpsMetricAlert resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Ready": the alert rule is synced with AxonOps and ready
	// - "Progressing": the alert rule is being created or updated
	// - "Failed": the alert rule failed to sync with AxonOps
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedAlertID is the ID of the alert rule assigned by AxonOps API
	// +optional
	SyncedAlertID string `json:"syncedAlertId,omitempty"`

	// CorrelationID is the UUID of the dashboard panel resolved from the dashboard template API
	// +optional
	CorrelationID string `json:"correlationId,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed AxonOpsMetricAlert
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="AlertID",type=string,JSONPath=`.status.syncedAlertId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsMetricAlert is the Schema for the axonopsmetricalerts API
type AxonOpsMetricAlert struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsMetricAlert
	// +required
	Spec AxonOpsMetricAlertSpec `json:"spec"`

	// status defines the observed state of AxonOpsMetricAlert
	// +optional
	Status AxonOpsMetricAlertStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsMetricAlertList contains a list of AxonOpsMetricAlert
type AxonOpsMetricAlertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsMetricAlert `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsMetricAlert{}, &AxonOpsMetricAlertList{})
}
