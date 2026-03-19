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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AxonOpsDashboardTemplateSpec defines the desired state of AxonOpsDashboardTemplate
type AxonOpsDashboardTemplateSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster this dashboard applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, dse, kafka)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;dse;kafka
	ClusterType string `json:"clusterType"`

	// DashboardName is the name of the dashboard in AxonOps. Used as the merge key
	// when updating the dashboard list on the remote API.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	DashboardName string `json:"dashboardName"`

	// Source specifies where the dashboard content comes from.
	// Exactly one of inline or configMapRef must be set.
	// +kubebuilder:validation:Required
	Source DashboardSource `json:"source"`
}

// DashboardSource specifies the source of the dashboard content.
// Exactly one of Inline or ConfigMapRef must be set.
type DashboardSource struct {
	// Inline contains the dashboard definition embedded directly in the CR.
	// +optional
	Inline *DashboardInline `json:"inline,omitempty"`

	// ConfigMapRef references a ConfigMap containing the dashboard JSON.
	// +optional
	ConfigMapRef *DashboardConfigMapRef `json:"configMapRef,omitempty"`
}

// DashboardInline holds an inline dashboard definition.
type DashboardInline struct {
	// Dashboard is an opaque JSON object containing the dashboard definition.
	// It should have optional "filters" and "panels" arrays matching the AxonOps
	// dashboard template API schema. Uses apiextensionsv1.JSON because kubebuilder
	// cannot generate OpenAPI schemas for json.RawMessage.
	//
	// Example:
	//   dashboard: {"filters": [...], "panels": [...]}
	// +kubebuilder:validation:Required
	Dashboard apiextensionsv1.JSON `json:"dashboard"`
}

// DashboardConfigMapRef references a ConfigMap containing dashboard JSON.
type DashboardConfigMapRef struct {
	// Name is the name of the ConfigMap in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key within the ConfigMap containing the dashboard JSON.
	// The value must be a JSON object with optional "filters" and "panels" arrays.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// AxonOpsDashboardTemplateStatus defines the observed state of AxonOpsDashboardTemplate.
type AxonOpsDashboardTemplateStatus struct {
	// Conditions represent the current state of the AxonOpsDashboardTemplate resource.
	//
	// Standard condition types include:
	// - "Ready": the dashboard is synced with AxonOps
	// - "Failed": the dashboard failed to sync with AxonOps
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// PanelCount is the number of panels in the last synced dashboard
	// +optional
	PanelCount int32 `json:"panelCount,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Dashboard",type=string,JSONPath=`.spec.dashboardName`
// +kubebuilder:printcolumn:name="Panels",type=integer,JSONPath=`.status.panelCount`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsDashboardTemplate is the Schema for the axonopsdashboardtemplates API
type AxonOpsDashboardTemplate struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsDashboardTemplate
	// +required
	Spec AxonOpsDashboardTemplateSpec `json:"spec"`

	// status defines the observed state of AxonOpsDashboardTemplate
	// +optional
	Status AxonOpsDashboardTemplateStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsDashboardTemplateList contains a list of AxonOpsDashboardTemplate
type AxonOpsDashboardTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsDashboardTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsDashboardTemplate{}, &AxonOpsDashboardTemplateList{})
}
