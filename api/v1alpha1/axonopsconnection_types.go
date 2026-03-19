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

// AxonOpsConnectionSpec defines the desired state of AxonOpsConnection
type AxonOpsConnectionSpec struct {
	// OrgID is the organization ID for AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	OrgID string `json:"orgId"`

	// APIKeyRef references a Secret containing the API key
	// +kubebuilder:validation:Required
	APIKeyRef AxonOpsSecretKeyRef `json:"apiKeyRef"`

	// Host is the AxonOps server hostname/URL (e.g., "dash.axonops.cloud" or "axonops.example.com")
	// If not provided, defaults to SaaS (dash.axonops.cloud)
	// +optional
	Host string `json:"host,omitempty"`

	// Protocol is the protocol to use (http or https)
	// If not provided, defaults to https
	// +optional
	// +kubebuilder:validation:Enum=http;https
	Protocol string `json:"protocol,omitempty"`

	// TokenType is the token type for authentication (Bearer or AxonApi)
	// If not provided, defaults to Bearer
	// +optional
	// +kubebuilder:validation:Enum=Bearer;AxonApi
	TokenType string `json:"tokenType,omitempty"`

	// TLSSkipVerify disables TLS certificate verification
	// Should only be used for testing
	// +optional
	TLSSkipVerify bool `json:"tlsSkipVerify,omitempty"`

	// UseSAML indicates if SAML is enabled (changes host URL pattern)
	// If not provided, defaults to false
	// +optional
	UseSAML bool `json:"useSaml,omitempty"`

	// Timeout is the HTTP client timeout for API requests (e.g. "30s", "2m").
	// If not provided, defaults to 30s. Recommended values: 30s for low-latency
	// networks, 60s-120s for cross-region or high-latency connections.
	// +optional
	Timeout string `json:"timeout,omitempty"`
}

// AxonOpsSecretKeyRef references a key in a Secret for AxonOps credentials
type AxonOpsSecretKeyRef struct {
	// Name is the name of the Secret in the same namespace
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key in the Secret containing the API key (default: "api_key")
	// +optional
	Key string `json:"key,omitempty"`
}

// AxonOpsConnectionStatus defines the observed state of AxonOpsConnection
type AxonOpsConnectionStatus struct {
	// Conditions represent the current state of the AxonOpsConnection resource
	// Standard condition types include:
	// - "Ready": the connection is valid and secret is readable
	// - "Failed": the connection failed (e.g., secret not found)
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastValidationTime is the timestamp of the last successful validation
	// +optional
	LastValidationTime *metav1.Time `json:"lastValidationTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed AxonOpsConnection
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=axconn
// +kubebuilder:printcolumn:name="OrgID",type=string,JSONPath=`.spec.orgId`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsConnection is the Schema for the axonopsconnections API
type AxonOpsConnection struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsConnection
	// +required
	Spec AxonOpsConnectionSpec `json:"spec"`

	// status defines the observed state of AxonOpsConnection
	// +optional
	Status AxonOpsConnectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsConnectionList contains a list of AxonOpsConnection
type AxonOpsConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsConnection{}, &AxonOpsConnectionList{})
}
