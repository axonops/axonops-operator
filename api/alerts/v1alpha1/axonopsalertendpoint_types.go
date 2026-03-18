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

// AxonOpsAlertEndpointSpec defines the desired state of AxonOpsAlertEndpoint.
// Manages integration definitions (Slack, PagerDuty, OpsGenie, ServiceNow, Teams, etc.)
// in the AxonOps API.
type AxonOpsAlertEndpointSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster this endpoint applies to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, kafka, dse)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;kafka;dse
	ClusterType string `json:"clusterType"`

	// Name is the human-readable name of the integration endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Type is the integration type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=slack;pagerduty;opsgenie;servicenow;microsoft_teams;email;smtp;webhook
	Type string `json:"type"`

	// Slack holds Slack-specific configuration. Must be set when type is "slack".
	// +optional
	Slack *SlackEndpointConfig `json:"slack,omitempty"`

	// PagerDuty holds PagerDuty-specific configuration. Must be set when type is "pagerduty".
	// +optional
	PagerDuty *PagerDutyEndpointConfig `json:"pagerDuty,omitempty"`

	// OpsGenie holds OpsGenie-specific configuration. Must be set when type is "opsgenie".
	// +optional
	OpsGenie *OpsGenieEndpointConfig `json:"opsGenie,omitempty"`

	// ServiceNow holds ServiceNow-specific configuration. Must be set when type is "servicenow".
	// +optional
	ServiceNow *ServiceNowEndpointConfig `json:"serviceNow,omitempty"`

	// MicrosoftTeams holds Microsoft Teams-specific configuration. Must be set when type is "microsoft_teams".
	// +optional
	MicrosoftTeams *MicrosoftTeamsEndpointConfig `json:"microsoftTeams,omitempty"`

	// Params holds generic key-value parameters for email, smtp, and webhook types.
	// Ignored for typed integrations (slack, pagerduty, opsgenie, servicenow, microsoft_teams).
	// +optional
	Params map[string]string `json:"params,omitempty"`
}

// SlackEndpointConfig defines Slack integration parameters.
type SlackEndpointConfig struct {
	// URL is the Slack incoming webhook URL (plain text).
	// +optional
	URL string `json:"url,omitempty"`

	// URLSecretRef references a Secret key containing the webhook URL.
	// Takes priority over URL when both are set.
	// +optional
	URLSecretRef *SecretKeyRef `json:"urlSecretRef,omitempty"`

	// Channel is the Slack channel name (optional override).
	// +optional
	Channel string `json:"channel,omitempty"`

	// AxonDashURL is the AxonOps dashboard URL included in messages.
	// +optional
	AxonDashURL string `json:"axondashUrl,omitempty"`
}

// PagerDutyEndpointConfig defines PagerDuty integration parameters.
type PagerDutyEndpointConfig struct {
	// IntegrationKey is the PagerDuty integration key (plain text).
	// +optional
	IntegrationKey string `json:"integrationKey,omitempty"`

	// IntegrationKeySecretRef references a Secret key containing the integration key.
	// Takes priority over IntegrationKey when both are set.
	// +optional
	IntegrationKeySecretRef *SecretKeyRef `json:"integrationKeySecretRef,omitempty"`
}

// OpsGenieEndpointConfig defines OpsGenie integration parameters.
type OpsGenieEndpointConfig struct {
	// OpsGenieKey is the OpsGenie API key (plain text).
	// +optional
	OpsGenieKey string `json:"opsgenieKey,omitempty"`

	// OpsGenieKeySecretRef references a Secret key containing the API key.
	// Takes priority over OpsGenieKey when both are set.
	// +optional
	OpsGenieKeySecretRef *SecretKeyRef `json:"opsgenieKeySecretRef,omitempty"`
}

// ServiceNowEndpointConfig defines ServiceNow integration parameters.
type ServiceNowEndpointConfig struct {
	// InstanceName is the ServiceNow instance name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceName string `json:"instanceName"`

	// User is the ServiceNow username.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// Password is the ServiceNow password (plain text).
	// +optional
	Password string `json:"password,omitempty"`

	// PasswordSecretRef references a Secret key containing the password.
	// Takes priority over Password when both are set.
	// +optional
	PasswordSecretRef *SecretKeyRef `json:"passwordSecretRef,omitempty"`
}

// MicrosoftTeamsEndpointConfig defines Microsoft Teams integration parameters.
type MicrosoftTeamsEndpointConfig struct {
	// WebHookURL is the Teams incoming webhook URL (plain text).
	// +optional
	WebHookURL string `json:"webHookURL,omitempty"`

	// WebHookURLSecretRef references a Secret key containing the webhook URL.
	// Takes priority over WebHookURL when both are set.
	// +optional
	WebHookURLSecretRef *SecretKeyRef `json:"webHookURLSecretRef,omitempty"`
}

// SecretKeyRef references a key within a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Secret in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key within the Secret data map.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// AxonOpsAlertEndpointStatus defines the observed state of AxonOpsAlertEndpoint.
type AxonOpsAlertEndpointStatus struct {
	// Conditions represent the current state of the AxonOpsAlertEndpoint resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedIntegrationID is the UUID assigned by the AxonOps API after creation
	// +optional
	SyncedIntegrationID string `json:"syncedIntegrationID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration is the generation observed at last reconciliation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Name",type=string,JSONPath=`.spec.name`
// +kubebuilder:printcolumn:name="IntegrationID",type=string,JSONPath=`.status.syncedIntegrationID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsAlertEndpoint is the Schema for the axonopsalertendpoints API.
// Manages integration endpoint definitions (Slack, PagerDuty, OpsGenie, ServiceNow, Teams, etc.)
// in the AxonOps API for alert notification delivery.
type AxonOpsAlertEndpoint struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsAlertEndpoint
	// +required
	Spec AxonOpsAlertEndpointSpec `json:"spec"`

	// status defines the observed state of AxonOpsAlertEndpoint
	// +optional
	Status AxonOpsAlertEndpointStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsAlertEndpointList contains a list of AxonOpsAlertEndpoint
type AxonOpsAlertEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsAlertEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsAlertEndpoint{}, &AxonOpsAlertEndpointList{})
}
