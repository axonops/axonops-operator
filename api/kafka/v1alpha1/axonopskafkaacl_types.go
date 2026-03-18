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

// AxonOpsKafkaACLSpec defines the desired state of AxonOpsKafkaACL.
// Each CR represents a single Kafka ACL entry identified by all fields combined.
type AxonOpsKafkaACLSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the Kafka cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ResourceType is the Kafka resource type for this ACL.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=ANY;TOPIC;GROUP;CLUSTER;TRANSACTIONAL_ID;DELEGATION_TOKEN;USER
	ResourceType string `json:"resourceType"`

	// ResourceName is the name of the Kafka resource (e.g., topic name, group name, "*" for all).
	// +kubebuilder:validation:Required
	ResourceName string `json:"resourceName"`

	// ResourcePatternType defines how the resource name is matched.
	// +kubebuilder:default="LITERAL"
	// +kubebuilder:validation:Enum=ANY;MATCH;LITERAL;PREFIXED
	// +optional
	ResourcePatternType string `json:"resourcePatternType,omitempty"`

	// Principal is the user or service identity (e.g., "User:alice", "User:*").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Principal string `json:"principal"`

	// Host is the client IP address. Defaults to "*" (all hosts).
	// +kubebuilder:default="*"
	// +optional
	Host string `json:"host,omitempty"`

	// Operation is the Kafka operation to allow or deny.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=ANY;ALL;READ;WRITE;CREATE;DELETE;ALTER;DESCRIBE;CLUSTER_ACTION;DESCRIBE_CONFIGS;ALTER_CONFIGS;IDEMPOTENT_WRITE;CREATE_TOKENS;DESCRIBE_TOKENS
	Operation string `json:"operation"`

	// PermissionType controls whether to allow or deny the operation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=ANY;DENY;ALLOW
	PermissionType string `json:"permissionType"`
}

// AxonOpsKafkaACLStatus defines the observed state of AxonOpsKafkaACL.
type AxonOpsKafkaACLStatus struct {
	// Conditions represent the current state of the resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Synced indicates the ACL has been created in the Kafka cluster
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
// +kubebuilder:resource:shortName=axacl
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Resource",type=string,JSONPath=`.spec.resourceType`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.resourceName`
// +kubebuilder:printcolumn:name="Principal",type=string,JSONPath=`.spec.principal`
// +kubebuilder:printcolumn:name="Operation",type=string,JSONPath=`.spec.operation`
// +kubebuilder:printcolumn:name="Permission",type=string,JSONPath=`.spec.permissionType`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// AxonOpsKafkaACL is the Schema for the axonopskafkaacls API.
// Manages Kafka ACL entries through the AxonOps API.
type AxonOpsKafkaACL struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsKafkaACLSpec `json:"spec"`

	// +optional
	Status AxonOpsKafkaACLStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsKafkaACLList contains a list of AxonOpsKafkaACL
type AxonOpsKafkaACLList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsKafkaACL `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsKafkaACL{}, &AxonOpsKafkaACLList{})
}
