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

// AxonOpsLogCollectorSpec defines the desired state of AxonOpsLogCollector.
// Manages log collector configurations for Cassandra/DSE clusters through the AxonOps API.
type AxonOpsLogCollectorSpec struct {
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
	// +kubebuilder:validation:Enum=cassandra;dse
	ClusterType string `json:"clusterType"`

	// Name is the display name of the log collector in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Filename is the log file path to collect. Used as the unique identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Filename string `json:"filename"`

	// Interval is the polling interval (e.g., "5s", "10s")
	// +kubebuilder:default="5s"
	// +optional
	Interval string `json:"interval,omitempty"`

	// Timeout is the operation timeout (e.g., "1m", "2m")
	// +kubebuilder:default="1m"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// DateFormat is the log date format string
	// +kubebuilder:default="yyyy-MM-dd HH:mm:ss,SSS"
	// +optional
	DateFormat string `json:"dateFormat,omitempty"`

	// InfoRegex is the regex pattern for INFO level log lines
	// +optional
	InfoRegex string `json:"infoRegex,omitempty"`

	// WarningRegex is the regex pattern for WARNING level log lines
	// +optional
	WarningRegex string `json:"warningRegex,omitempty"`

	// ErrorRegex is the regex pattern for ERROR level log lines
	// +optional
	ErrorRegex string `json:"errorRegex,omitempty"`

	// DebugRegex is the regex pattern for DEBUG level log lines
	// +optional
	DebugRegex string `json:"debugRegex,omitempty"`

	// Readonly enables read-only mode for the log collector
	// +optional
	Readonly bool `json:"readonly,omitempty"`
}

// AxonOpsLogCollectorStatus defines the observed state of AxonOpsLogCollector.
type AxonOpsLogCollectorStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedUUID is the UUID of the collector in AxonOps
	// +optional
	SyncedUUID string `json:"syncedUUID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation most recently observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=axlog
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Filename",type=string,JSONPath=`.spec.filename`
// +kubebuilder:printcolumn:name="Interval",type=string,JSONPath=`.spec.interval`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsLogCollector is the Schema for the axonopslogcollectors API.
// Manages log collector configurations through the AxonOps API.
type AxonOpsLogCollector struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsLogCollectorSpec `json:"spec"`

	// +optional
	Status AxonOpsLogCollectorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsLogCollectorList contains a list of AxonOpsLogCollector
type AxonOpsLogCollectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsLogCollector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsLogCollector{}, &AxonOpsLogCollectorList{})
}
