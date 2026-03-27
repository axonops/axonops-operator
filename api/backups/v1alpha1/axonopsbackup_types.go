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

// AxonOpsBackupSpec defines the desired state of AxonOpsBackup.
// Manages Cassandra scheduled snapshots through the AxonOps API.
type AxonOpsBackupSpec struct {
	// ConnectionRef is the name of an AxonOpsConnection in the same namespace.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConnectionRef string `json:"connectionRef"`

	// ClusterName is the Cassandra/DSE cluster name in AxonOps
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ClusterName string `json:"clusterName"`

	// ClusterType is the cluster type. Only cassandra and dse support backups.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=cassandra;dse
	ClusterType string `json:"clusterType"`

	// Tag uniquely identifies this backup configuration within the cluster.
	// Used by the controller to match existing backups in AxonOps for idempotent updates.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Tag string `json:"tag"`

	// Datacenters to include in the backup
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	// +listType=atomic
	Datacenters []string `json:"datacenters"`

	// Schedule enables scheduled (cron-based) backups. Defaults to true.
	// +kubebuilder:default=true
	// +optional
	Schedule *bool `json:"schedule,omitempty"`

	// ScheduleExpression is the cron expression for scheduling (e.g., "0 1 * * *" for daily at 1am)
	// +kubebuilder:default="0 1 * * *"
	// +optional
	ScheduleExpression string `json:"scheduleExpression,omitempty"`

	// LocalRetention is how long to keep local snapshots (e.g., "10d", "24h", "2w").
	// Passed directly to the AxonOps API.
	// +kubebuilder:default="10d"
	// +optional
	LocalRetention string `json:"localRetention,omitempty"`

	// Timeout is the maximum duration for the backup operation (e.g., "10h")
	// +kubebuilder:default="10h"
	// +optional
	Timeout string `json:"timeout,omitempty"`

	// Nodes optionally limits the backup to specific node names. Empty means all nodes.
	// +optional
	// +listType=atomic
	Nodes []string `json:"nodes,omitempty"`

	// Keyspaces optionally limits the backup to specific keyspaces.
	// Mutually exclusive with Tables.
	// +optional
	// +listType=atomic
	Keyspaces []string `json:"keyspaces,omitempty"`

	// Tables optionally limits the backup to specific tables in "keyspace.table" format.
	// Mutually exclusive with Keyspaces.
	// +optional
	// +listType=atomic
	Tables []string `json:"tables,omitempty"`

	// Remote configures remote storage for backup offloading.
	// When nil, backups are local-only.
	// +optional
	Remote *RemoteBackupConfig `json:"remote,omitempty"`
}

// RemoteBackupConfig defines remote storage settings for backups.
type RemoteBackupConfig struct {
	// Type is the remote storage backend
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=s3;sftp;azure
	Type string `json:"type"`

	// Path is the remote storage path/prefix
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// Retention is the duration to keep remote backups (e.g., "60d")
	// +kubebuilder:default="60d"
	// +optional
	Retention string `json:"retention,omitempty"`

	// Transfers is the number of parallel file transfers
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=1
	// +optional
	Transfers int `json:"transfers,omitempty"`

	// TPSLimit is the transactions-per-second limit during transfer
	// +kubebuilder:default=50
	// +optional
	TPSLimit int `json:"tpsLimit,omitempty"`

	// BandwidthLimit restricts transfer bandwidth (e.g., "100M")
	// +optional
	BandwidthLimit string `json:"bandwidthLimit,omitempty"`

	// S3 configures AWS S3 storage (required when type=s3)
	// +optional
	S3 *BackupS3Config `json:"s3,omitempty"`

	// SFTP configures SFTP storage (required when type=sftp)
	// +optional
	SFTP *BackupSFTPConfig `json:"sftp,omitempty"`

	// Azure configures Azure Blob storage (required when type=azure)
	// +optional
	Azure *BackupAzureConfig `json:"azure,omitempty"`
}

// BackupS3Config defines AWS S3 remote backup settings.
// Credential precedence: CredentialsRef > AccessKeyID/SecretAccessKey > IAM (env_auth=true)
type BackupS3Config struct {
	// Region is the AWS region
	// +kubebuilder:validation:Required
	Region string `json:"region"`

	// CredentialsRef references a Secret containing keys "access_key_id" and "secret_access_key".
	// Takes precedence over inline credentials. If neither is set, IAM auth is used.
	// +optional
	CredentialsRef string `json:"credentialsRef,omitempty"`

	// AccessKeyID is the AWS access key ID (inline, for dev or vals-operator use).
	// Ignored if CredentialsRef is set.
	// +optional
	AccessKeyID string `json:"accessKeyID,omitempty"`

	// SecretAccessKey is the AWS secret access key (inline, for dev or vals-operator use).
	// Ignored if CredentialsRef is set.
	// +optional
	SecretAccessKey string `json:"secretAccessKey,omitempty"`

	// StorageClass is the S3 storage class
	// +kubebuilder:default="STANDARD"
	// +kubebuilder:validation:Enum=STANDARD;reduced_redundancy;standard_ia;onezone_ia;glacier;deep_archive;intelligent_tiering
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// ACL is the S3 canned access control list
	// +kubebuilder:default="private"
	// +kubebuilder:validation:Enum=private;public-read;public-read-write;authenticated-read;bucket-owner-read
	// +optional
	ACL string `json:"acl,omitempty"`

	// Encryption is the server-side encryption method
	// +kubebuilder:default="AES256"
	// +kubebuilder:validation:Enum=none;AES256
	// +optional
	Encryption string `json:"encryption,omitempty"`

	// NoCheckBucket disables bucket existence verification
	// +optional
	NoCheckBucket bool `json:"noCheckBucket,omitempty"`

	// DisableChecksum disables checksum verification during upload
	// +optional
	DisableChecksum bool `json:"disableChecksum,omitempty"`
}

// BackupSFTPConfig defines SFTP remote backup settings.
// Credential precedence: CredentialsRef > inline User/Password/KeyFile
type BackupSFTPConfig struct {
	// Host is the SFTP server address (hostname or hostname:port)
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// CredentialsRef references a Secret containing keys "ssh_user", "ssh_pass", and/or "key_file".
	// Takes precedence over inline credentials.
	// +optional
	CredentialsRef string `json:"credentialsRef,omitempty"`

	// User is the SSH username (inline). Ignored if CredentialsRef is set.
	// +optional
	User string `json:"user,omitempty"`

	// Password is the SSH password (inline). Ignored if CredentialsRef is set.
	// +optional
	Password string `json:"password,omitempty"`

	// KeyFile is the path to the SSH private key (inline). Ignored if CredentialsRef is set.
	// +optional
	KeyFile string `json:"keyFile,omitempty"`
}

// BackupAzureConfig defines Azure Blob remote backup settings.
// Credential precedence: CredentialsRef > inline Key > MSI
type BackupAzureConfig struct {
	// Account is the Azure storage account name
	// +kubebuilder:validation:Required
	Account string `json:"account"`

	// CredentialsRef references a Secret containing key "azure_key".
	// Takes precedence over inline Key. If neither is set and UseMSI is true, MSI is used.
	// +optional
	CredentialsRef string `json:"credentialsRef,omitempty"`

	// Key is the Azure storage account key (inline). Ignored if CredentialsRef is set.
	// +optional
	Key string `json:"key,omitempty"`

	// Endpoint is a custom Azure Blob endpoint
	// +optional
	Endpoint string `json:"endpoint,omitempty"`

	// UseMSI enables Azure Managed Service Identity authentication
	// +optional
	UseMSI bool `json:"useMSI,omitempty"`

	// MSIObjectID is the MSI object ID (mutually exclusive with MSIClientID and MSIResourceID)
	// +optional
	MSIObjectID string `json:"msiObjectID,omitempty"`

	// MSIClientID is the MSI client ID (mutually exclusive with MSIObjectID and MSIResourceID)
	// +optional
	MSIClientID string `json:"msiClientID,omitempty"`

	// MSIResourceID is the MSI resource ID (mutually exclusive with MSIObjectID and MSIClientID)
	// +optional
	MSIResourceID string `json:"msiResourceID,omitempty"`
}

// AxonOpsBackupStatus defines the observed state of AxonOpsBackup.
type AxonOpsBackupStatus struct {
	// Conditions represent the current state of the AxonOpsBackup resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// SyncedBackupID is the ID assigned by the AxonOps API
	// +optional
	SyncedBackupID string `json:"syncedBackupID,omitempty"`

	// LastSyncTime is the timestamp of the last successful sync with AxonOps
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation most recently observed
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=axbkp
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterName`
// +kubebuilder:printcolumn:name="Tag",type=string,JSONPath=`.spec.tag`
// +kubebuilder:printcolumn:name="Remote",type=string,JSONPath=`.spec.remote.type`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AxonOpsBackup is the Schema for the axonopsbackups API.
// Manages Cassandra scheduled snapshot backups through the AxonOps API.
type AxonOpsBackup struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec AxonOpsBackupSpec `json:"spec"`

	// +optional
	Status AxonOpsBackupStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// AxonOpsBackupList contains a list of AxonOpsBackup
type AxonOpsBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []AxonOpsBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AxonOpsBackup{}, &AxonOpsBackupList{})
}
