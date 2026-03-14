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

// AxonOpsHealthcheckShellSpec defines the desired state of AxonOpsHealthcheckShell
type AxonOpsHealthcheckShellSpec struct {
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

	// Script is the script or command to execute (e.g., /usr/bin/ls, /path/to/script.sh)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Script string `json:"script"`

	// Shell is the shell to use for executing the script (e.g., /bin/bash)
	// +kubebuilder:validation:Optional
	Shell string `json:"shell,omitempty"`

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
}

// AxonOpsHealthcheckShellStatus defines the observed state of AxonOpsHealthcheckShell.
type AxonOpsHealthcheckShellStatus struct {
	// Conditions represent the current state of the AxonOpsHealthcheckShell resource.
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
// +kubebuilder:printcolumn:name="Script",type=string,JSONPath=`.spec.script`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// AxonOpsHealthcheckShell is the Schema for the axonopshealthcheckshells API
type AxonOpsHealthcheckShell struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsHealthcheckShell
	// +required
	Spec AxonOpsHealthcheckShellSpec `json:"spec"`

	// status defines the observed state of AxonOpsHealthcheckShell
	// +optional
	Status AxonOpsHealthcheckShellStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsHealthcheckShellList contains a list of AxonOpsHealthcheckShell
type AxonOpsHealthcheckShellList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsHealthcheckShell `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsHealthcheckShell{}, &AxonOpsHealthcheckShellList{})
}
