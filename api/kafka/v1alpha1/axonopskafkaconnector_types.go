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

// AxonOpsKafkaConnectorSpec defines the desired state of AxonOpsKafkaConnector.
type AxonOpsKafkaConnectorSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the Kafka cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ConnectClusterName is the Kafka Connect cluster name within the Kafka cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectClusterName string `json:"connectClusterName"`

	// Name is the connector name. Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Config contains the connector configuration as key-value pairs.
	// Must include "connector.class" at minimum.
	// +kubebuilder:validation:Required
	Config map[string]string `json:"config"`
}

// AxonOpsKafkaConnectorStatus defines the observed state of AxonOpsKafkaConnector.
type AxonOpsKafkaConnectorStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ConnectorType is the type reported by Kafka Connect ("source" or "sink")
	// +optional
	ConnectorType string `json:"connectorType,omitempty"`

	// Synced indicates the connector has been created in the Kafka Connect cluster
	// +optional
	Synced bool `json:"synced,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation most recently observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=axconn
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Connect",type=string,JSONPath=`.spec.connectClusterName`
// +kubebuilder:printcolumn:name="Connector",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.status.connectorType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsKafkaConnector is the Schema for the axonopskafkaconnectors API.
// Manages Kafka Connect connectors through the AxonOps API.
type AxonOpsKafkaConnector struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsKafkaConnectorSpec `json:"spec"`

	// +optional
	Status AxonOpsKafkaConnectorStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsKafkaConnectorList contains a list of AxonOpsKafkaConnector
type AxonOpsKafkaConnectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsKafkaConnector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsKafkaConnector{}, &AxonOpsKafkaConnectorList{})
}
