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

// AxonOpsCommitlogArchiveSpec defines the desired state of AxonOpsCommitlogArchive
type AxonOpsCommitlogArchiveSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the name of the cluster
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the type of the cluster (cassandra, dse)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;dse
	ClusterType string `json:"clusterType"`

	// RemoteType is the storage backend type for commitlog archiving
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=local;s3;sftp
	RemoteType string `json:"remoteType"`

	// RemotePath is the storage path (e.g. s3://bucket/path, /mnt/backups)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RemotePath string `json:"remotePath"`

	// RemoteRetention is the retention duration for archived commitlogs (e.g. "60d", "90d")
	// +optional
	// +kubebuilder:default="60d"
	RemoteRetention string `json:"remoteRetention,omitempty"`

	// Timeout is the archive operation timeout (e.g. "10h", "24h")
	// +optional
	// +kubebuilder:default="10h"
	Timeout string `json:"timeout,omitempty"`

	// Transfers is the number of parallel transfers (0 = unlimited)
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	Transfers *int32 `json:"transfers,omitempty"`

	// BwLimit is the bandwidth limit (e.g. "10M", empty = unlimited)
	// +optional
	BwLimit string `json:"bwLimit,omitempty"`

	// Datacenters is the list of datacenters where archiving is active
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Datacenters []string `json:"datacenters"`

	// S3 contains S3-specific configuration. Required when remoteType is "s3".
	// +optional
	S3 *CommitlogS3Config `json:"s3,omitempty"`

	// SFTP contains SFTP-specific configuration. Required when remoteType is "sftp".
	// +optional
	SFTP *CommitlogSFTPConfig `json:"sftp,omitempty"`
}

// CommitlogS3Config contains S3-specific settings for commitlog archiving
type CommitlogS3Config struct {
	// Region is the AWS region (e.g. "us-east-1")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`

	// CredentialsRef references a Secret containing "access_key_id" and "secret_access_key" keys.
	// If omitted, IAM role/environment auth is used.
	// +optional
	CredentialsRef *CommitlogSecretRef `json:"credentialsRef,omitempty"`

	// StorageClass is the S3 storage class (default: STANDARD)
	// +optional
	// +kubebuilder:default="STANDARD"
	StorageClass string `json:"storageClass,omitempty"`

	// ACL is the S3 object ACL (default: private)
	// +optional
	// +kubebuilder:default="private"
	ACL string `json:"acl,omitempty"`

	// Encryption is the server-side encryption type: "none" or "AES256" (default: AES256)
	// +optional
	// +kubebuilder:default="AES256"
	// +kubebuilder:validation:Enum=none;AES256
	Encryption string `json:"encryption,omitempty"`

	// DisableChecksum disables S3 checksum validation
	// +optional
	DisableChecksum bool `json:"disableChecksum,omitempty"`
}

// CommitlogSecretRef references a Secret for credentials
type CommitlogSecretRef struct {
	// Name is the Secret name in the same namespace
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// CommitlogSFTPConfig contains SFTP-specific settings for commitlog archiving
type CommitlogSFTPConfig struct {
	// Host is the SFTP hostname
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`

	// User is the SSH username
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`

	// CredentialsRef references a Secret containing SSH credentials.
	// Use passwordKey for password auth or keyFileKey for key-based auth.
	// +optional
	CredentialsRef *CommitlogSFTPCredentialsRef `json:"credentialsRef,omitempty"`
}

// CommitlogSFTPCredentialsRef references a Secret with SFTP credentials
type CommitlogSFTPCredentialsRef struct {
	// Name is the Secret name in the same namespace
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// PasswordKey is the key in the Secret for the SSH password
	// +optional
	PasswordKey string `json:"passwordKey,omitempty"`

	// KeyFileKey is the key in the Secret for the SSH private key content
	// +optional
	KeyFileKey string `json:"keyFileKey,omitempty"`
}

// AxonOpsCommitlogArchiveStatus defines the observed state of AxonOpsCommitlogArchive.
type AxonOpsCommitlogArchiveStatus struct {
	// Conditions represent the current state of the resource.
	// Condition types: "Ready", "Failed"
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.clusterType`
// +kubebuilder:printcolumn:name="Remote",type=string,JSONPath=`.spec.remoteType`
// +kubebuilder:printcolumn:name="Path",type=string,JSONPath=`.spec.remotePath`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsCommitlogArchive is the Schema for the axonopscommitlogarchives API
type AxonOpsCommitlogArchive struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of AxonOpsCommitlogArchive
	// +required
	Spec AxonOpsCommitlogArchiveSpec `json:"spec"`

	// status defines the observed state of AxonOpsCommitlogArchive
	// +optional
	Status AxonOpsCommitlogArchiveStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsCommitlogArchiveList contains a list of AxonOpsCommitlogArchive
type AxonOpsCommitlogArchiveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsCommitlogArchive `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsCommitlogArchive{}, &AxonOpsCommitlogArchiveList{})
}
