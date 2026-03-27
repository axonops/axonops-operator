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

package backups

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	backupsv1alpha1 "github.com/axonops/axonops-operator/api/backups/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	backupFinalizerName = "backups.axonops.com/backup-finalizer"
	condTypeReady       = "Ready"
)

// AxonOpsBackupReconciler reconciles a AxonOpsBackup object
type AxonOpsBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=backups.axonops.com,resources=axonopsbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=backups.axonops.com,resources=axonopsbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=backups.axonops.com,resources=axonopsbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsBackup
func (r *AxonOpsBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.backup", trace.WithAttributes())
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	start := time.Now()
	defer func() {
		resultStr := axonopsmetrics.ResultSuccess
		if err != nil {
			resultStr = axonopsmetrics.ResultError
			axonopsmetrics.ReconcileErrorsTotal.WithLabelValues(axonopsmetrics.ClassifyError(err)).Inc()
		}
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsbackup", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsbackup", resultStr).Inc()
	}()

	backup := &backupsv1alpha1.AxonOpsBackup{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsBackup", "backup", req.NamespacedName)

	// Handle deletion
	if backup.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, backup)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(backup, backupFinalizerName) {
		controllerutil.AddFinalizer(backup, backupFinalizerName)
		if err := r.Update(ctx, backup); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Validate mutual exclusivity of tables and keyspaces
	if len(backup.Spec.Tables) > 0 && len(backup.Spec.Keyspaces) > 0 {
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: backup.Generation,
			Reason:             "ValidationError",
			Message:            "Tables and Keyspaces are mutually exclusive — specify one or neither",
		})
		backup.Status.ObservedGeneration = backup.Generation
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client
	apiClient, err := common.ResolveAPIClient(ctx, r.Client, backup.Namespace, backup.Spec.ConnectionRef)
	if errors.Is(err, common.ErrConnectionPaused) {
		return common.HandleConnectionPaused(ctx, r.Client, backup, &backup.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: backup.Generation,
			Reason:             "FailedToResolveConnection",
			Message:            common.SafeConditionMsg("Failed to resolve connection", err),
		})
		backup.Status.ObservedGeneration = backup.Generation
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(backup.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		backup.Status.ObservedGeneration == backup.Generation &&
		backup.Status.SyncedBackupID != "" {
		log.Info("Backup already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// Build the backup payload
	payload, err := r.buildBackupPayload(ctx, backup)
	if err != nil {
		log.Error(err, "Failed to build backup payload")
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: backup.Generation,
			Reason:             "PayloadBuildError",
			Message:            common.SafeConditionMsg("Failed to build backup payload", err),
		})
		backup.Status.ObservedGeneration = backup.Generation
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if backup already exists by tag (for update: delete + recreate)
	if backup.Status.SyncedBackupID != "" {
		log.Info("Deleting existing backup before recreate", "backupID", backup.Status.SyncedBackupID)
		if err := apiClient.DeleteScheduledSnapshot(ctx, backup.Spec.ClusterType, backup.Spec.ClusterName, backup.Status.SyncedBackupID); err != nil {
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			// Non-retryable (e.g., 404) — proceed with create
		}
	}

	// Create the backup
	if err := apiClient.CreateScheduledSnapshot(ctx, backup.Spec.ClusterType, backup.Spec.ClusterName, payload); err != nil {
		log.Error(err, "Failed to create scheduled snapshot")
		meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: backup.Generation,
			Reason:             "SyncFailed",
			Message:            common.SafeConditionMsg("Failed to sync with AxonOps", err),
		})
		backup.Status.ObservedGeneration = backup.Generation
		if err := r.Status().Update(ctx, backup); err != nil {
			log.Error(err, "Failed to update status")
		}
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Re-fetch CR and update status
	syncedID := payload.ID
	syncedGeneration := backup.Generation
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	backup.Status.SyncedBackupID = syncedID
	now := metav1.Now()
	backup.Status.LastSyncTime = &now
	backup.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Backup schedule synced with AxonOps",
	})
	meta.RemoveStatusCondition(&backup.Status.Conditions, "Progressing")
	meta.RemoveStatusCondition(&backup.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, backup); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced backup schedule", "backupID", syncedID, "tag", backup.Spec.Tag)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsBackupReconciler) handleDeletion(ctx context.Context, backup *backupsv1alpha1.AxonOpsBackup) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(backup, backupFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting backup from AxonOps", "backupID", backup.Status.SyncedBackupID, "tag", backup.Spec.Tag)

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, backup.Namespace, backup.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if backup.Status.SyncedBackupID != "" {
		if err := apiClient.DeleteScheduledSnapshot(ctx, backup.Spec.ClusterType, backup.Spec.ClusterName, backup.Status.SyncedBackupID); err != nil {
			log.Error(err, "Failed to delete backup from AxonOps", "backupID", backup.Status.SyncedBackupID)
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted backup from AxonOps API", "backupID", backup.Status.SyncedBackupID)
		}
	} else {
		log.Info("SyncedBackupID is empty, skipping API deletion")
	}

	controllerutil.RemoveFinalizer(backup, backupFinalizerName)
	if err := r.Update(ctx, backup); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildBackupPayload constructs the AxonOps API payload from the CR spec
func (r *AxonOpsBackupReconciler) buildBackupPayload(ctx context.Context, backup *backupsv1alpha1.AxonOpsBackup) (axonops.BackupPayload, error) {
	// Build tables list
	var tables []axonops.TableRef
	for _, t := range backup.Spec.Tables {
		parts := strings.SplitN(t, ".", 2)
		if len(parts) == 2 {
			tables = append(tables, axonops.TableRef{Name: parts[1]})
		}
	}

	schedule := true
	if backup.Spec.Schedule != nil {
		schedule = *backup.Spec.Schedule
	}

	payload := axonops.BackupPayload{
		ID:                     backup.Status.SyncedBackupID,
		Tag:                    backup.Spec.Tag,
		Datacenters:            backup.Spec.Datacenters,
		Nodes:                  backup.Spec.Nodes,
		Tables:                 tables,
		Keyspaces:              backup.Spec.Keyspaces,
		AllTables:              len(tables) == 0,
		AllNodes:               len(backup.Spec.Nodes) == 0,
		LocalRetentionDuration: backup.Spec.LocalRetention,
		Timeout:                backup.Spec.Timeout,
		Schedule:               schedule,
		ScheduleExpr:           backup.Spec.ScheduleExpression,
	}

	if payload.ID == "" {
		payload.ID = uuid.New().String()
	}
	if payload.Nodes == nil {
		payload.Nodes = []string{}
	}
	if payload.Keyspaces == nil {
		payload.Keyspaces = []string{}
	}
	if payload.Tables == nil {
		payload.Tables = []axonops.TableRef{}
	}

	// Remote configuration
	if backup.Spec.Remote != nil {
		remote := backup.Spec.Remote
		payload.Remote = true
		payload.RemoteType = remote.Type
		payload.RemotePath = remote.Path
		payload.RemoteRetentionDuration = remote.Retention
		payload.Transfers = remote.Transfers
		payload.TPSLimit = float64(remote.TPSLimit)
		payload.BWLimit = remote.BandwidthLimit

		remoteConfig, err := r.buildRemoteConfig(ctx, backup)
		if err != nil {
			return axonops.BackupPayload{}, fmt.Errorf("failed to build remote config: %w", err)
		}
		payload.RemoteConfig = remoteConfig
	}

	return payload, nil
}

// buildRemoteConfig builds the rclone-style newline-delimited key=value config string
func (r *AxonOpsBackupReconciler) buildRemoteConfig(ctx context.Context, backup *backupsv1alpha1.AxonOpsBackup) (string, error) {
	remote := backup.Spec.Remote
	if remote == nil {
		return "", nil
	}

	switch remote.Type {
	case "s3":
		return r.buildS3RemoteConfig(ctx, backup.Namespace, remote.S3)
	case "sftp":
		return r.buildSFTPRemoteConfig(ctx, backup.Namespace, remote.SFTP)
	case "azure":
		return r.buildAzureRemoteConfig(ctx, backup.Namespace, remote.Azure)
	default:
		return "", fmt.Errorf("unsupported remote type: %s", remote.Type)
	}
}

func (r *AxonOpsBackupReconciler) buildS3RemoteConfig(ctx context.Context, namespace string, s3 *backupsv1alpha1.BackupS3Config) (string, error) {
	if s3 == nil {
		return "", fmt.Errorf("s3 config is required when remote type is s3")
	}

	// Resolve credentials: SecretRef > inline > IAM
	accessKeyID, secretAccessKey, err := r.resolveS3Credentials(ctx, namespace, s3)
	if err != nil {
		return "", err
	}

	config := map[string]string{
		"type":                   "s3",
		"provider":               "AWS",
		"region":                 s3.Region,
		"acl":                    s3.ACL,
		"server_side_encryption": s3.Encryption,
		"storage_class":          s3.StorageClass,
		"no_check_bucket":        boolToString(s3.NoCheckBucket),
		"disable_checksum":       boolToString(s3.DisableChecksum),
	}

	if accessKeyID != "" && secretAccessKey != "" {
		config["env_auth"] = "false"
		config["access_key_id"] = accessKeyID
		config["secret_access_key"] = secretAccessKey
	} else {
		config["env_auth"] = trueStr
	}

	return formatRemoteConfig(config), nil
}

func (r *AxonOpsBackupReconciler) buildSFTPRemoteConfig(ctx context.Context, namespace string, sftp *backupsv1alpha1.BackupSFTPConfig) (string, error) {
	if sftp == nil {
		return "", fmt.Errorf("sftp config is required when remote type is sftp")
	}

	user, pass, keyFile, err := r.resolveSFTPCredentials(ctx, namespace, sftp)
	if err != nil {
		return "", err
	}

	config := map[string]string{
		"type":     "sftp",
		"host":     sftp.Host,
		"user":     user,
		"pass":     pass,
		"key_file": keyFile,
	}

	return formatRemoteConfig(config), nil
}

func (r *AxonOpsBackupReconciler) buildAzureRemoteConfig(ctx context.Context, namespace string, azure *backupsv1alpha1.BackupAzureConfig) (string, error) {
	if azure == nil {
		return "", fmt.Errorf("azure config is required when remote type is azure")
	}

	key, err := r.resolveAzureCredentials(ctx, namespace, azure)
	if err != nil {
		return "", err
	}

	config := map[string]string{
		"type":    "azureblob",
		"account": azure.Account,
	}

	if key != "" {
		config["key"] = key
	}
	if azure.Endpoint != "" {
		config["endpoint"] = azure.Endpoint
	}
	if azure.UseMSI {
		config["use_msi"] = trueStr
		if azure.MSIObjectID != "" {
			config["msi_object_id"] = azure.MSIObjectID
		}
		if azure.MSIClientID != "" {
			config["msi_client_id"] = azure.MSIClientID
		}
		if azure.MSIResourceID != "" {
			config["msi_mi_res_id"] = azure.MSIResourceID
		}
	}

	return formatRemoteConfig(config), nil
}

// resolveS3Credentials resolves S3 credentials following precedence: SecretRef > inline > empty (IAM)
func (r *AxonOpsBackupReconciler) resolveS3Credentials(ctx context.Context, namespace string, s3 *backupsv1alpha1.BackupS3Config) (string, string, error) {
	if s3.CredentialsRef != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: s3.CredentialsRef, Namespace: namespace}, secret); err != nil {
			return "", "", fmt.Errorf("failed to get S3 credentials secret %q: %w", s3.CredentialsRef, err)
		}
		return string(secret.Data["access_key_id"]), string(secret.Data["secret_access_key"]), nil
	}
	if s3.AccessKeyID != "" && s3.SecretAccessKey != "" {
		return s3.AccessKeyID, s3.SecretAccessKey, nil
	}
	return "", "", nil // IAM auth
}

// resolveSFTPCredentials resolves SFTP credentials following precedence: SecretRef > inline
func (r *AxonOpsBackupReconciler) resolveSFTPCredentials(ctx context.Context, namespace string, sftp *backupsv1alpha1.BackupSFTPConfig) (string, string, string, error) {
	if sftp.CredentialsRef != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: sftp.CredentialsRef, Namespace: namespace}, secret); err != nil {
			return "", "", "", fmt.Errorf("failed to get SFTP credentials secret %q: %w", sftp.CredentialsRef, err)
		}
		return string(secret.Data["ssh_user"]), string(secret.Data["ssh_pass"]), string(secret.Data["key_file"]), nil
	}
	return sftp.User, sftp.Password, sftp.KeyFile, nil
}

// resolveAzureCredentials resolves Azure credentials following precedence: SecretRef > inline > empty (MSI)
func (r *AxonOpsBackupReconciler) resolveAzureCredentials(ctx context.Context, namespace string, azure *backupsv1alpha1.BackupAzureConfig) (string, error) {
	if azure.CredentialsRef != "" {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: azure.CredentialsRef, Namespace: namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to get Azure credentials secret %q: %w", azure.CredentialsRef, err)
		}
		return string(secret.Data["azure_key"]), nil
	}
	if azure.Key != "" {
		return azure.Key, nil
	}
	return "", nil // MSI auth
}

// formatRemoteConfig builds a rclone-style newline-delimited "key = value" string.
// Keys are sorted for deterministic output.
func formatRemoteConfig(config map[string]string) string {
	// Use sorted keys for deterministic output
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	// Sort alphabetically — not required by AxonOps API but ensures hash stability
	slices.Sort(keys)

	var parts []string
	for _, k := range keys {
		if config[k] != "" {
			parts = append(parts, fmt.Sprintf("%s = %s", k, config[k]))
		}
	}
	return strings.Join(parts, "\n")
}

const (
	trueStr  = "true"
	falseStr = "false"
)

func boolToString(b bool) string {
	if b {
		return trueStr
	}
	return falseStr
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&backupsv1alpha1.AxonOpsBackup{}).
		Named("backups-axonopsbackup").
		Complete(r)
}
