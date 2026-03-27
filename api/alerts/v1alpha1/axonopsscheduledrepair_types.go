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

// AxonOpsScheduledRepairSpec defines the desired state of AxonOpsScheduledRepair.
// Manages Cassandra scheduled repair configurations through the AxonOps API.
type AxonOpsScheduledRepairSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the Cassandra/DSE cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the cluster type. Only cassandra and dse support scheduled repairs.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;dse
	ClusterType string `json:"clusterType"`

	// Tag uniquely identifies this repair configuration within the cluster.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Tag string `json:"tag"`

	// ScheduleExpression is the cron expression for scheduling (e.g., "0 0 1 * *" for monthly)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ScheduleExpression string `json:"scheduleExpression"`

	// Keyspace to repair. Empty means all keyspaces.
	// +optional
	Keyspace string `json:"keyspace,omitempty"`

	// Tables to repair within the keyspace. Empty means all tables.
	// +optional
	// +listType=atomic
	Tables []string `json:"tables,omitempty"`

	// BlacklistedTables are tables to exclude from repair.
	// +optional
	// +listType=atomic
	BlacklistedTables []string `json:"blacklistedTables,omitempty"`

	// Nodes limits repair to specific nodes. Empty means all nodes.
	// +optional
	// +listType=atomic
	Nodes []string `json:"nodes,omitempty"`

	// SpecificDataCenters limits repair to specific data centers. Empty means all.
	// +optional
	// +listType=atomic
	SpecificDataCenters []string `json:"specificDataCenters,omitempty"`

	// Parallelism controls the repair execution mode.
	// +kubebuilder:default="Parallel"
	// +kubebuilder:validation:Enum=Parallel;Sequential;"DC-Aware"
	// +optional
	Parallelism string `json:"parallelism,omitempty"`

	// Segmented enables segmented repair.
	// +optional
	Segmented bool `json:"segmented,omitempty"`

	// SegmentsPerNode is the number of segments per node for segmented repair.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	SegmentsPerNode int `json:"segmentsPerNode,omitempty"`

	// Incremental enables incremental repair.
	// +optional
	Incremental bool `json:"incremental,omitempty"`

	// JobThreads is the number of repair threads.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	JobThreads int `json:"jobThreads,omitempty"`

	// PrimaryRange enables primary range repair only.
	// +optional
	PrimaryRange bool `json:"primaryRange,omitempty"`

	// OptimiseStreams enables repair stream optimisation.
	// +optional
	OptimiseStreams bool `json:"optimiseStreams,omitempty"`

	// SkipPaxos skips Paxos repair. Mutually exclusive with PaxosOnly.
	// +optional
	SkipPaxos bool `json:"skipPaxos,omitempty"`

	// PaxosOnly runs Paxos repair only. Mutually exclusive with SkipPaxos.
	// +optional
	PaxosOnly bool `json:"paxosOnly,omitempty"`
}

// AxonOpsScheduledRepairStatus defines the observed state of AxonOpsScheduledRepair.
type AxonOpsScheduledRepairStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedRepairID is the ID assigned by the AxonOps API
	// +optional
	SyncedRepairID string `json:"syncedRepairID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation most recently observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=axrep
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Tag",type=string,JSONPath=`.spec.tag`
// +kubebuilder:printcolumn:name="Schedule",type=string,JSONPath=`.spec.scheduleExpression`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsScheduledRepair is the Schema for the axonopsscheduledrepairs API.
// Manages Cassandra scheduled repairs through the AxonOps API.
type AxonOpsScheduledRepair struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsScheduledRepairSpec `json:"spec"`

	// +optional
	Status AxonOpsScheduledRepairStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsScheduledRepairList contains a list of AxonOpsScheduledRepair
type AxonOpsScheduledRepairList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsScheduledRepair `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsScheduledRepair{}, &AxonOpsScheduledRepairList{})
}
