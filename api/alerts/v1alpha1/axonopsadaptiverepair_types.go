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

// AxonOpsAdaptiveRepairSpec defines the desired state of AxonOpsAdaptiveRepair
type AxonOpsAdaptiveRepairSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster this adaptive repair configuration applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, dse). Kafka is not supported for adaptive repair.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;dse
	ClusterType string `json:"clusterType"`

	// Active enables or disables adaptive repair for the cluster
	// +optional
	// +kubebuilder:default=true
	Active *bool `json:"active,omitempty"`

	// GcGraceThreshold is the minimum gc_grace_seconds value for a table to be considered for repair.
	// Tables with gc_grace shorter than this value will not be repaired.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=86400
	GcGraceThreshold *int64 `json:"gcGraceThreshold,omitempty"`

	// TableParallelism is the number of tables repaired concurrently
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	TableParallelism *int32 `json:"tableParallelism,omitempty"`

	// ExcludedTables is a list of fully-qualified table names excluded from repair (e.g. keyspace.table).
	// Maps to the API field BlacklistedTables.
	// +optional
	// +listType=atomic
	ExcludedTables []string `json:"excludedTables,omitempty"`

	// FilterTWCSTables skips tables using TimeWindowCompactionStrategy when set to true
	// +optional
	// +kubebuilder:default=true
	FilterTWCSTables *bool `json:"filterTWCSTables,omitempty"`

	// SegmentRetries is the maximum number of retries per repair segment before marking it as failed
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=3
	SegmentRetries *int32 `json:"segmentRetries,omitempty"`

	// SegmentTargetSizeMB is the target size in MB for each repair segment. 0 means AxonOps decides automatically.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	SegmentTargetSizeMB *int32 `json:"segmentTargetSizeMB,omitempty"`

	// SegmentTimeout is the per-segment timeout duration string (e.g. "2h", "30m")
	// +optional
	// +kubebuilder:default="2h"
	SegmentTimeout string `json:"segmentTimeout,omitempty"`

	// MaxSegmentsPerTable is the maximum number of segments per table. 0 means AxonOps decides automatically.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	MaxSegmentsPerTable *int32 `json:"maxSegmentsPerTable,omitempty"`
}

// AxonOpsAdaptiveRepairStatus defines the observed state of AxonOpsAdaptiveRepair.
type AxonOpsAdaptiveRepairStatus struct {
	// Conditions represent the current state of the AxonOpsAdaptiveRepair resource.
	//
	// Standard condition types include:
	// - "Ready": the adaptive repair settings are synced with AxonOps
	// - "Failed": the settings failed to sync with AxonOps
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed AxonOpsAdaptiveRepair
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.spec.active`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsAdaptiveRepair is the Schema for the axonopsadaptiverepairs API
type AxonOpsAdaptiveRepair struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsAdaptiveRepair
	// +required
	Spec AxonOpsAdaptiveRepairSpec `json:"spec"`

	// status defines the observed state of AxonOpsAdaptiveRepair
	// +optional
	Status AxonOpsAdaptiveRepairStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsAdaptiveRepairList contains a list of AxonOpsAdaptiveRepair
type AxonOpsAdaptiveRepairList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsAdaptiveRepair `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsAdaptiveRepair{}, &AxonOpsAdaptiveRepairList{})
}
