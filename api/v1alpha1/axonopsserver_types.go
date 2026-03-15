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

// Ingress defines an ingress configuration for the AxonOps Workbench.
type Ingress struct {
	// Enabled specifies whether an Ingress resource should be created for the Workbench.
	Enabled bool `json:"enabled,omitempty"`

	// ApiVersion allows overriding the default networking.k8s.io/v1 API version if necessary.
	// +optional
	ApiVersion string `json:"apiVersion,omitempty"`

	// Annotations are custom annotations to be added to the Ingress resource.
	// This is often used for cert-manager (e.g., cert-manager.io/issuer).
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels are custom labels to be added to the Ingress resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// IngressClassName specifies the IngressClass cluster resource that should handle this Ingress.
	// +optional
	IngressClassName string `json:"ingressClassName,omitempty"`

	// Hosts is a list of hostnames that the Ingress will rule over.
	// +kubebuilder:validation:MinItems=1
	Hosts []string `json:"hosts,omitempty"`

	// TLS configuration for the Ingress. If cert-manager is used,
	// this defines the secret names where certificates will be stored.
	// +optional
	TLS []networkingv1.IngressTLS `json:"tls,omitempty"`

	// Path is the URL path that the Ingress will route to the service.
	// +kubebuilder:default="/"
	// +optional
	Path string `json:"path,omitempty"`

	// PathType determines how the Ingress path matching is performed.
	// Supported values are Exact, Prefix, and ImplementationSpecific.
	// +kubebuilder:validation:Enum=Exact;Prefix;ImplementationSpecific
	// +kubebuilder:default=Prefix
	PathType networkingv1.PathType `json:"pathType,omitempty"`

	// ServiceName is the name of the Kubernetes Service to route traffic to.
	ServiceName string `json:"serviceName,omitempty"`

	// ServicePort is the port number of the service to route traffic to.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ServicePort int32 `json:"servicePort,omitempty"`
}

// GatewayConfig defines the configuration for exposing the AxonOps Workbench
// using the Kubernetes Gateway API.
type GatewayConfig struct {
	// Enabled specifies whether Gateway API resources (Gateway and HTTPRoute)
	// should be created for the Workbench.
	Enabled bool `json:"enabled,omitempty"`

	// GatewayClassName is the name of the GatewayClass that should manage the Gateway.
	// Common values include 'istio', 'nginx', or 'traefik'.
	// +kubebuilder:validation:MinLength=1
	// +optional
	GatewayClassName string `json:"gatewayClassName,omitempty"`

	// Annotations are custom annotations to be added to the Gateway resource.
	// To enable automated TLS, include cert-manager annotations like 'cert-manager.io/issuer'.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels are custom labels to be added to the Gateway and HTTPRoute resources.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Port is the network port the Gateway listener will stay open on.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=443
	Port int32 `json:"port,omitempty"`

	// Hostname is the DNS name used for the Gateway listener and HTTPRoute matching.
	// For example: 'workbench.example.com'.
	// +kubebuilder:validation:MinLength=1
	Hostname string `json:"hostname,omitempty"`

	// Path is the URL prefix used for routing traffic to the Workbench service.
	// +kubebuilder:default="/"
	// +optional
	Path string `json:"path,omitempty"`

	// ServiceName is the name of the backend Kubernetes Service to route traffic to.
	ServiceName string `json:"serviceName,omitempty"`

	// ServicePort is the port of the backend Service to route traffic to.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ServicePort int32 `json:"servicePort,omitempty"`
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

	// OrgName configures to organization
	// +required
	OrgName string `json:"orgName"`

	// License configures licence key or secret reference
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless

	License License `json:"license,omitempty"`

	// AgentIngress configures ingress for agent connections
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AgentIngress Ingress `json:"agentIngress,omitempty"`

	// ApiIngress configures ingress for API access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ApiIngress Ingress `json:"apiIngress,omitempty"`

	// AgentGateway configures gateway for agent connections
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	AgentGateway GatewayConfig `json:"agentGateway,omitempty"`

	// ApiGateway configures ingress for API access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	ApiGateway GatewayConfig `json:"apiGateway,omitempty"`
}

type License struct {
	Key       string `json:"key,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
}

// AxonDashboardComponent adds Ingress on top of the base fields.
type AxonDashboardComponent struct {
	AxonBaseComponent `json:",inline"`

	// Replicas configures the number of dashboard replicas
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:default=1

	Replicas *int32 `json:"replicas,omitempty"`

	// Ingress configures ingress for dashboard access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Ingress Ingress `json:"ingress,omitempty"`

	// Gateway configures GatewayAPI for dashboard access
	// +optional
	// +kubebuilder:validation:XPreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Gateway GatewayConfig `json:"gateway,omitempty"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsServer{}, &AxonOpsServerList{})
}
