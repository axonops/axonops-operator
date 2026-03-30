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

// AxonOpsSilenceWindowSpec defines the desired state of AxonOpsSilenceWindow.
// Manages alert silence windows through the AxonOps API.
type AxonOpsSilenceWindowSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the cluster type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;kafka;dse
	ClusterType string `json:"clusterType"`

	// Duration is how long the silence lasts (e.g., "1h", "30m", "3h")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Duration string `json:"duration"`

	// Active enables or disables the silence window. Defaults to true.
	// When set to false, an existing silence is deleted from the API.
	// +kubebuilder:default=true
	// +optional
	Active *bool `json:"active,omitempty"`

	// Recurring enables recurring silence on a cron schedule.
	// +optional
	Recurring bool `json:"recurring,omitempty"`

	// CronExpression defines the schedule for recurring silences (e.g., "0 1 * * *").
	// Also used as the matching key to identify existing silences in the API.
	// +kubebuilder:default="0 * * * *"
	// +optional
	CronExpression string `json:"cronExpression,omitempty"`

	// Datacenters optionally limits silence to specific data centers.
	// Empty means all data centers.
	// +optional
	// +listType=atomic
	Datacenters []string `json:"datacenters,omitempty"`
}

// AxonOpsSilenceWindowStatus defines the observed state of AxonOpsSilenceWindow.
type AxonOpsSilenceWindowStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedSilenceID is the ID assigned by the AxonOps API
	// +optional
	SyncedSilenceID string `json:"syncedSilenceID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation most recently observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=axsil
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.spec.duration`
// +kubebuilder:printcolumn:name="Recurring",type=boolean,JSONPath=`.spec.recurring`
// +kubebuilder:printcolumn:name="Cron",type=string,JSONPath=`.spec.cronExpression`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsSilenceWindow is the Schema for the axonopssilencewindows API.
// Manages alert silence windows through the AxonOps API.
type AxonOpsSilenceWindow struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsSilenceWindowSpec `json:"spec"`

	// +optional
	Status AxonOpsSilenceWindowStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsSilenceWindowList contains a list of AxonOpsSilenceWindow
type AxonOpsSilenceWindowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsSilenceWindow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsSilenceWindow{}, &AxonOpsSilenceWindowList{})
}
