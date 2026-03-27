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

// AxonOpsHealthcheckHTTPSpec defines the desired state of AxonOpsHealthcheckHTTP
type AxonOpsHealthcheckHTTPSpec struct {
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

	// URL is the HTTP URL to check
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`

	// Method is the HTTP method to use (GET, POST, PUT, DELETE, etc.)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="GET"
	Method string `json:"method,omitempty"`

	// Headers is a map of HTTP headers to send with the request
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the HTTP request body (for POST, PUT, etc.)
	// +kubebuilder:validation:Optional
	Body string `json:"body,omitempty"`

	// ExpectedStatus is the expected HTTP response status code
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=200
	ExpectedStatus int `json:"expectedStatus,omitempty"`

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

	// TLSSkipVerify skips TLS certificate verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	TLSSkipVerify bool `json:"tlsSkipVerify,omitempty"`

	// SupportedAgentTypes lists agent types this healthcheck applies to
	// +kubebuilder:validation:Optional
	// +listType=atomic
	SupportedAgentTypes []string `json:"supportedAgentTypes,omitempty"`
}

// AxonOpsHealthcheckHTTPStatus defines the observed state of AxonOpsHealthcheckHTTP.
type AxonOpsHealthcheckHTTPStatus struct {
	// Conditions represent the current state of the AxonOpsHealthcheckHTTP resource.
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
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

// AxonOpsHealthcheckHTTP is the Schema for the axonopshealthcheckhttps API
type AxonOpsHealthcheckHTTP struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsHealthcheckHTTP
	// +required
	Spec AxonOpsHealthcheckHTTPSpec `json:"spec"`

	// status defines the observed state of AxonOpsHealthcheckHTTP
	// +optional
	Status AxonOpsHealthcheckHTTPStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsHealthcheckHTTPList contains a list of AxonOpsHealthcheckHTTP
type AxonOpsHealthcheckHTTPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsHealthcheckHTTP `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsHealthcheckHTTP{}, &AxonOpsHealthcheckHTTPList{})
}
