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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AxonOpsServerSpec defines the desired state of AxonOpsServer
// All components are enabled by default. Set component.enabled=false to disable.
type AxonOpsServerSpec struct {
	// Server configures the axon-server component
	// +optional
	Server *AxonServerComponent `json:"server,omitempty"`

	// TimeSeries configures the axondb-timeseries component
	// +optional
	TimeSeries *AxonDbComponent `json:"timeSeries,omitempty"`

	// Search configures the axondb-search component
	// +optional
	Search *AxonDbComponent `json:"search,omitempty"`

	// Dashboard configures the axon-dash component
	// +optional
	Dashboard *AxonDashboardComponent `json:"dashboard,omitempty"`
}

// AxonOpsServerStatus defines the observed state of AxonOpsServer.
type AxonOpsServerStatus struct {
	// Conditions represent the current state of the AxonOpsServer resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// TimeSeriesSecretName is the name of the Secret containing TimeSeries credentials.
	// This is either the user-provided SecretRef or an auto-generated secret name.
	// +optional
	TimeSeriesSecretName string `json:"timeSeriesSecretName,omitempty"`

	// SearchSecretName is the name of the Secret containing Search credentials.
	// This is either the user-provided SecretRef or an auto-generated secret name.
	// +optional
	SearchSecretName string `json:"searchSecretName,omitempty"`

	// TimeSeriesCertSecretName is the name of the Secret containing the TLS certificate for TimeSeries.
	// +optional
	TimeSeriesCertSecretName string `json:"timeSeriesCertSecretName,omitempty"`

	// SearchCertSecretName is the name of the Secret containing the TLS certificate for Search.
	// +optional
	SearchCertSecretName string `json:"searchCertSecretName,omitempty"`

	// ObservedGeneration reflects the generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// AxonOpsServer is the Schema for the axonopsservers API; it's the top-level resource for AxonOpsSever, TimeSeries and SearchDB
type AxonOpsServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsServer
	// +required
	Spec AxonOpsServerSpec `json:"spec"`

	// status defines the observed state of AxonOpsServer
	// +optional
	Status AxonOpsServerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsServerList contains a list of AxonOpsServer
type AxonOpsServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsServer `json:"items"`
}

// AxonAuthentication configures database authentication credentials.
// Priority: SecretRef > Username/Password > Auto-generated
type AxonAuthentication struct {
	// SecretRef references an existing Secret containing AXONOPS_DB_USER and AXONOPS_DB_PASSWORD keys.
	// If set, Username and Password fields are ignored.
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// Username for database authentication. If empty and SecretRef is not set,
	// a random username will be generated.
	// +optional
	Username string `json:"username,omitempty"`

	// Password for database authentication. If empty and SecretRef is not set,
	// a random password will be generated.
	// +optional
	Password string `json:"password,omitempty"`
}

// AxonExternalConfig configures external access to the component
type AxonExternalConfig struct {
	// Hosts is a list of external hostnames for the component
	// +optional
	Hosts []string `json:"hosts,omitempty"`

	// TLS configures TLS settings for external connections
	// +optional
	TLS AxonTLSConfig `json:"tls,omitempty"`
}

// AxonTLSConfig configures TLS settings
type AxonTLSConfig struct {
	// Enabled enables TLS for connections
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// InsecureSkipVerify skips TLS certificate verification
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// AxonRepository configures the container image
type AxonRepository struct {
	// Image is the container image name
	// +optional
	Image string `json:"image,omitempty"`

	// Tag is the container image tag
	// +optional
	Tag string `json:"tag,omitempty"`
	// PullPolicy is the image pull policy (e.g., "Always", "IfNotPresent", "Never")
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
}

type AxonBaseComponent struct {
	// Enabled determines if the component should be deployed
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Annotations to add to the component pods
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels to add to the component pods
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Env is a list of additional environment variables
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Env []corev1.EnvVar `json:"env,omitempty"`

	// External configures external access to the component
	// +optional
	External AxonExternalConfig `json:"external,omitempty"`

	// Authentication configures database credentials.
	// If not specified, random credentials will be generated automatically.
	// +optional
	Authentication AxonAuthentication `json:"authentication,omitempty"`

	// HeapSize configures the JVM heap size (e.g., "1024M", "4G")
	// +optional
	// +kubebuilder:default="1024M"
	HeapSize string `json:"heapSize,omitempty"`

	// Repository configures the container image
	// +optional
	Repository AxonRepository `json:"repository,omitempty"`

	// Resources defines compute resources for the component
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// ExtraVolumes defines additional volumes to mount
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`

	// ExtraVolumeMounts defines additional volume mounts
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// StorageConfig defines persistent storage configuration
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	StorageConfig corev1.PersistentVolumeClaimSpec `json:"storageConfig,omitempty"`
}

// AxonDbComponent uses the base fields as-is.
type AxonDbComponent struct {
	AxonBaseComponent `json:",inline"`
}

// AxonServerComponent adds Ingress on top of the base fields.
type AxonServerComponent struct {
	AxonBaseComponent `json:",inline"`

	// AgentIngress configures ingress for agent connections
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AgentIngress networkingv1.IngressSpec `json:"agentIngress,omitempty"`

	// ApiIngress configures ingress for API access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ApiIngress networkingv1.IngressSpec `json:"apiIngress,omitempty"`
}

// AxonDashboardComponent adds Ingress on top of the base fields.
type AxonDashboardComponent struct {
	AxonBaseComponent `json:",inline"`

	// Ingress configures ingress for dashboard access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Ingress networkingv1.IngressSpec `json:"ingress,omitempty"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsServer{}, &AxonOpsServerList{})
}
