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

// AxonOpsKafkaTopicSpec defines the desired state of AxonOpsKafkaTopic.
type AxonOpsKafkaTopicSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the Kafka cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// Name is the Kafka topic name. Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Partitions is the number of partitions. Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	Partitions int32 `json:"partitions"`

	// ReplicationFactor is the replication factor. Immutable after creation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	ReplicationFactor int32 `json:"replicationFactor"`

	// Config contains Kafka topic configuration overrides.
	// Keys use dot notation (e.g., "cleanup.policy", "retention.ms").
	// Only explicitly set configs are managed; unset keys use Kafka broker defaults.
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

// AxonOpsKafkaTopicStatus defines the observed state of AxonOpsKafkaTopic.
type AxonOpsKafkaTopicStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Synced indicates the topic has been created in the Kafka cluster
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
// +kubebuilder:resource:shortName=axkt
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Topic",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="Partitions",type=integer,JSONPath=`.spec.partitions`
// +kubebuilder:printcolumn:name="RF",type=integer,JSONPath=`.spec.replicationFactor`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsKafkaTopic is the Schema for the axonopskafkatopics API.
// Manages Kafka topics through the AxonOps API.
type AxonOpsKafkaTopic struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsKafkaTopicSpec `json:"spec"`

	// +optional
	Status AxonOpsKafkaTopicStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsKafkaTopicList contains a list of AxonOpsKafkaTopic
type AxonOpsKafkaTopicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsKafkaTopic `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsKafkaTopic{}, &AxonOpsKafkaTopicList{})
}
