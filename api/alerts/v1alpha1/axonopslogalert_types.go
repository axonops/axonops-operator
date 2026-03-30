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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// AxonOpsLogAlertSpec defines the desired state of AxonOpsLogAlert
type AxonOpsLogAlertSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

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

	// Content is the message or phrase to match in logs
	// +optional
	Content string `json:"content,omitempty"`

	// Level specifies log levels to match (comma-separated: error, warning, info, debug)
	// +optional
	Level string `json:"level,omitempty"`

	// LogType specifies the type of logs to match (e.g., system, application)
	// +optional
	LogType string `json:"logType,omitempty"`

	// Source specifies the log file source path (e.g., /var/log/cassandra/system.log)
	// +optional
	Source string `json:"source,omitempty"`

	// Filters apply constraints to the alert based on infrastructure dimensions
	// +optional
	Filters *LogAlertFilters `json:"filters,omitempty"`

	// Annotations contains optional alert metadata
	// +optional
	Annotations *LogAlertAnnotations `json:"annotations,omitempty"`

	// Integrations specifies how to route alert notifications
	// DEPRECATED: This field is not implemented and is ignored by the controller.
	// Use the AxonOpsAlertRoute CRD instead for alert routing and notification configuration.
	// This field is retained for potential future use but should not be configured.
	// +optional
	Integrations *LogAlertIntegration `json:"integrations,omitempty"`
}

// LogAlertFilters defines optional constraints for alert matching by infrastructure dimensions
type LogAlertFilters struct {
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
}

// LogAlertAnnotations defines optional alert metadata
type LogAlertAnnotations struct {
	// Summary is a short summary of the alert
	// +optional
	Summary string `json:"summary,omitempty"`

	// Description is a longer description of the alert
	// +optional
	Description string `json:"description,omitempty"`
}

// LogAlertIntegration defines alert notification routing
type LogAlertIntegration struct {
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

// AxonOpsLogAlertStatus defines the observed state of AxonOpsLogAlert.
type AxonOpsLogAlertStatus struct {
	// Conditions represent the current state of the AxonOpsLogAlert resource.
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
	SyncedAlertID string `json:"syncedAlertID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed AxonOpsLogAlert
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="AlertID",type=string,JSONPath=`.status.syncedAlertID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsLogAlert is the Schema for the axonopslogalerts API
type AxonOpsLogAlert struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsLogAlert
	// +required
	Spec AxonOpsLogAlertSpec `json:"spec"`

	// status defines the observed state of AxonOpsLogAlert
	// +optional
	Status AxonOpsLogAlertStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsLogAlertList contains a list of AxonOpsLogAlert
type AxonOpsLogAlertList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsLogAlert `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsLogAlert{}, &AxonOpsLogAlertList{})
}
