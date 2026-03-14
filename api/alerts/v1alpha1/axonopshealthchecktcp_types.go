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

// AxonOpsHealthcheckTCPSpec defines the desired state of AxonOpsHealthcheckTCP
type AxonOpsHealthcheckTCPSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, kafka, dse)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;kafka;dse
	ClusterType string `json:"clusterType"`

	// Name is the name of the healthcheck
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// TCP is the TCP address to check (e.g., 0.0.0.0:9092)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TCP string `json:"tcp"`

	// Interval is the interval between checks (e.g., 1m, 30s)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1m"
	Interval string `json:"interval,omitempty"`

	// Timeout is the timeout for the check (e.g., 1m, 30s)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1m"
	Timeout string `json:"timeout,omitempty"`

	// Readonly indicates whether the healthcheck is read-only
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Readonly bool `json:"readonly,omitempty"`

	// SupportedAgentTypes lists agent types this healthcheck applies to
	// +kubebuilder:validation:Optional
	// +listType=atomic
	SupportedAgentTypes []string `json:"supportedAgentTypes,omitempty"`
}

// AxonOpsHealthcheckTCPStatus defines the observed state of AxonOpsHealthcheckTCP.
type AxonOpsHealthcheckTCPStatus struct {
	// Conditions represent the current state of the AxonOpsHealthcheckTCP resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedHealthcheckID is the ID assigned by AxonOps API
	// +optional
	SyncedHealthcheckID string `json:"syncedHealthcheckID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="TCP",type=string,JSONPath=`.spec.tcp`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// AxonOpsHealthcheckTCP is the Schema for the axonopshealthchecktcps API
type AxonOpsHealthcheckTCP struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsHealthcheckTCP
	// +required
	Spec AxonOpsHealthcheckTCPSpec `json:"spec"`

	// status defines the observed state of AxonOpsHealthcheckTCP
	// +optional
	Status AxonOpsHealthcheckTCPStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsHealthcheckTCPList contains a list of AxonOpsHealthcheckTCP
type AxonOpsHealthcheckTCPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsHealthcheckTCP `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsHealthcheckTCP{}, &AxonOpsHealthcheckTCPList{})
}
