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

// AxonOpsAlertRouteSpec defines the desired state of AxonOpsAlertRoute.
// Routes alert notifications to integrations (Slack, PagerDuty, email, etc.)
type AxonOpsAlertRouteSpec struct {
	// ConnectionRef references the AxonOpsConnection for API credentials
	// +kubebuilder:validation:Required
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster
	// +kubebuilder:validation:Required
	ClusterName string `json:"clusterName"`

	// ClusterType is the cluster type (cassandra, kafka, or dse)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;kafka;dse
	ClusterType string `json:"clusterType"`

	// IntegrationName is the name of the integration
	// +kubebuilder:validation:Required
	IntegrationName string `json:"integrationName"`

	// IntegrationType is the type of integration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=email;smtp;pagerduty;slack;teams;servicenow;webhook;opsgenie
	IntegrationType string `json:"integrationType"`

	// Type is the route type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=global;metrics;backups;servicechecks;nodes;commands;repairs;rollingrestart
	Type string `json:"type"`

	// Severity is the alert severity level
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=info;warning;error
	Severity string `json:"severity"`

	// EnableOverride enables override for non-global routes.
	// Ignored for global routes. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	EnableOverride bool `json:"enableOverride,omitempty"`
}

// AxonOpsAlertRouteStatus defines the observed state of AxonOpsAlertRoute.
type AxonOpsAlertRouteStatus struct {
	// Conditions represent the current state of the AxonOpsAlertRoute resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// IntegrationID is the resolved ID of the integration from the AxonOps API
	// +optional
	IntegrationID string `json:"integrationId,omitempty"`

	// LastSyncTime is the last time the route was successfully synced with the AxonOps API
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=`.spec.severity`
// +kubebuilder:printcolumn:name="Integration",type=string,JSONPath=`.spec.integrationName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsAlertRoute is the Schema for the axonopsalertroutes API.
// Routes alert notifications from AxonOps to integrations (Slack, PagerDuty, email, etc.)
type AxonOpsAlertRoute struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsAlertRoute
	// +required
	Spec AxonOpsAlertRouteSpec `json:"spec"`

	// status defines the observed state of AxonOpsAlertRoute
	// +optional
	Status AxonOpsAlertRouteStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsAlertRouteList contains a list of AxonOpsAlertRoute
type AxonOpsAlertRouteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsAlertRoute `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsAlertRoute{}, &AxonOpsAlertRouteList{})
}
