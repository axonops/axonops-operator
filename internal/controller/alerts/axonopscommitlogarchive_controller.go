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

package alerts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	commitlogArchiveFinalizer = "alerts.axonops.com/commitlog-archive-finalizer"
)

// AxonOpsCommitlogArchiveReconciler reconciles a AxonOpsCommitlogArchive object
type AxonOpsCommitlogArchiveReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopscommitlogarchives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopscommitlogarchives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopscommitlogarchives/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsCommitlogArchive
func (r *AxonOpsCommitlogArchiveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.commitlogarchive", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopscommitlogarchive", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopscommitlogarchive", resultStr).Inc()
	}()

	archive := &alertsv1alpha1.AxonOpsCommitlogArchive{}
	if err := r.Get(ctx, req.NamespacedName, archive); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsCommitlogArchive", "archive", req.NamespacedName)

	if archive.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, archive)
	}

	if !controllerutil.ContainsFinalizer(archive, commitlogArchiveFinalizer) {
		controllerutil.AddFinalizer(archive, commitlogArchiveFinalizer)
		if err := r.Update(ctx, archive); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := ResolveAPIClient(ctx, r.Client, archive.Namespace, archive.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, archive, &archive.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, archive, ReasonConnectionError, fmt.Sprintf("Failed to resolve connection: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	readyCond := meta.FindStatusCondition(archive.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		archive.Status.ObservedGeneration == archive.Generation &&
		archive.Status.LastSyncTime != nil {
		log.Info("Commitlog archive already synced and spec unchanged, skipping")
		return ctrl.Result{}, nil
	}

	if err := r.validateConfig(archive); err != nil {
		r.setFailedCondition(ctx, archive, "InvalidConfig", err.Error())
		return ctrl.Result{}, nil
	}

	payload, err := r.buildPayload(ctx, archive)
	if err != nil {
		r.setFailedCondition(ctx, archive, "SecretNotFound", fmt.Sprintf("Failed to build payload: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	existing, err := apiClient.GetCommitlogArchiveSettings(ctx, archive.Spec.ClusterType, archive.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get commitlog archive settings")
		r.setFailedCondition(ctx, archive, ReasonAPIError, fmt.Sprintf("Failed to get settings: %v", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && !apiErr.IsRetryable() {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// DELETE existing settings for this remoteType before POST (API uses DELETE+POST for updates)
	for _, s := range existing {
		if s.RemoteType == archive.Spec.RemoteType {
			log.Info("Deleting existing commitlog archive settings before update", "remoteType", archive.Spec.RemoteType)
			if err := apiClient.DeleteCommitlogArchiveSettings(ctx, archive.Spec.ClusterType, archive.Spec.ClusterName, archive.Spec.Datacenters); err != nil {
				log.Error(err, "Failed to delete existing settings")
				r.setFailedCondition(ctx, archive, ReasonAPIError, fmt.Sprintf("Failed to delete existing settings: %v", err))
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				return ctrl.Result{}, nil
			}
			break
		}
	}

	if err := apiClient.CreateCommitlogArchiveSettings(ctx, archive.Spec.ClusterType, archive.Spec.ClusterName, payload); err != nil {
		log.Error(err, "Failed to create commitlog archive settings")
		r.setFailedCondition(ctx, archive, ReasonAPIError, fmt.Sprintf("Failed to create settings: %v", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	syncedGeneration := archive.Generation
	if err := r.Get(ctx, req.NamespacedName, archive); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := metav1.Now()
	archive.Status.LastSyncTime = &now
	archive.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&archive.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Commitlog archive settings synced with AxonOps",
	})
	meta.RemoveStatusCondition(&archive.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, archive); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced commitlog archive settings", "remoteType", archive.Spec.RemoteType)
	return ctrl.Result{}, nil
}

func (r *AxonOpsCommitlogArchiveReconciler) validateConfig(archive *alertsv1alpha1.AxonOpsCommitlogArchive) error {
	switch archive.Spec.RemoteType {
	case "s3":
		if archive.Spec.S3 == nil {
			return fmt.Errorf("s3 config block is required when remoteType is s3")
		}
	case "sftp":
		if archive.Spec.SFTP == nil {
			return fmt.Errorf("sftp config block is required when remoteType is sftp")
		}
	}
	return nil
}

func (r *AxonOpsCommitlogArchiveReconciler) buildPayload(ctx context.Context, archive *alertsv1alpha1.AxonOpsCommitlogArchive) (axonops.CommitlogArchivePayload, error) {
	transfers := int32(0)
	if archive.Spec.Transfers != nil {
		transfers = *archive.Spec.Transfers
	}
	payload := axonops.CommitlogArchivePayload{
		BackupMethod:            "Incremental",
		Timeout:                 archive.Spec.Timeout,
		BwLimit:                 archive.Spec.BwLimit,
		RemoteRetentionDuration: archive.Spec.RemoteRetention,
		RemoteType:              archive.Spec.RemoteType,
		RemotePath:              archive.Spec.RemotePath,
		Datacenters:             archive.Spec.Datacenters,
		Transfers:               transfers,
	}

	var configParts []string

	switch archive.Spec.RemoteType {
	case "local":
		configParts = append(configParts, "type = local")

	case "s3":
		s3 := archive.Spec.S3
		configParts = append(configParts,
			"type = s3",
			"provider = AWS",
			fmt.Sprintf("region = %s", s3.Region),
			fmt.Sprintf("acl = %s", s3.ACL),
			fmt.Sprintf("server_side_encryption = %s", s3.Encryption),
			fmt.Sprintf("storage_class = %s", s3.StorageClass),
			fmt.Sprintf("disable_checksum = %v", s3.DisableChecksum),
		)
		payload.AWSRegion = s3.Region
		payload.AWSStorageClass = s3.StorageClass
		payload.AWSACL = s3.ACL
		payload.AWSServerSideEncryption = s3.Encryption
		payload.AWSDisableChecksum = s3.DisableChecksum

		if s3.CredentialsRef != nil {
			secret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: archive.Namespace, Name: s3.CredentialsRef.Name}, secret); err != nil {
				return payload, fmt.Errorf("failed to get S3 credentials secret %q: %w", s3.CredentialsRef.Name, err)
			}
			accessKey := string(secret.Data["access_key_id"])
			secretKey := string(secret.Data["secret_access_key"])
			configParts = append(configParts, "env_auth = false",
				fmt.Sprintf("access_key_id = %s", accessKey),
				fmt.Sprintf("s3_secret_access_key = %s", secretKey))
			payload.AWSAccessKeyID = accessKey
			payload.AWSSecretAccessKey = secretKey
		} else {
			configParts = append(configParts, "env_auth = true")
		}

	case "sftp":
		sftp := archive.Spec.SFTP
		configParts = append(configParts,
			"type = sftp",
			fmt.Sprintf("host = %s", sftp.Host),
			fmt.Sprintf("ssh_user = %s", sftp.User),
		)
		payload.SFTPHost = &sftp.Host
		payload.SFTPUser = &sftp.User

		if sftp.CredentialsRef != nil {
			secret := &corev1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: archive.Namespace, Name: sftp.CredentialsRef.Name}, secret); err != nil {
				return payload, fmt.Errorf("failed to get SFTP credentials secret %q: %w", sftp.CredentialsRef.Name, err)
			}
			if sftp.CredentialsRef.PasswordKey != "" {
				pass := string(secret.Data[sftp.CredentialsRef.PasswordKey])
				configParts = append(configParts, fmt.Sprintf("ssh_pass = %s", pass))
				payload.SFTPPass = &pass
			}
			if sftp.CredentialsRef.KeyFileKey != "" {
				keyFile := string(secret.Data[sftp.CredentialsRef.KeyFileKey])
				configParts = append(configParts, fmt.Sprintf("key_file = %s", keyFile))
				payload.SFTPKeyFile = &keyFile
			}
		}
	}

	payload.RemoteConfig = strings.Join(configParts, "\n")
	return payload, nil
}

func (r *AxonOpsCommitlogArchiveReconciler) handleDeletion(ctx context.Context, archive *alertsv1alpha1.AxonOpsCommitlogArchive) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(archive, commitlogArchiveFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting commitlog archive settings from AxonOps", "remoteType", archive.Spec.RemoteType)

	apiClient, err := ResolveAPIClient(ctx, r.Client, archive.Namespace, archive.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if err := apiClient.DeleteCommitlogArchiveSettings(ctx, archive.Spec.ClusterType, archive.Spec.ClusterName, archive.Spec.Datacenters); err != nil {
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || !apiErr.IsRetryable()) {
			log.Info("Settings not found or non-retryable error, proceeding with finalizer removal")
		} else {
			log.Error(err, "Failed to delete commitlog archive settings — will retry")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else {
		log.Info("Successfully deleted commitlog archive settings from AxonOps")
	}

	controllerutil.RemoveFinalizer(archive, commitlogArchiveFinalizer)
	if err := r.Update(ctx, archive); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AxonOpsCommitlogArchiveReconciler) setFailedCondition(ctx context.Context, archive *alertsv1alpha1.AxonOpsCommitlogArchive, reason, message string) {
	meta.SetStatusCondition(&archive.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: archive.Generation,
		Reason:             reason,
		Message:            message,
	})
	archive.Status.ObservedGeneration = archive.Generation
	if err := r.Status().Update(ctx, archive); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsCommitlogArchiveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsCommitlogArchive{}).
		Named("alerts-axonopscommitlogarchive").
		Complete(r)
}
