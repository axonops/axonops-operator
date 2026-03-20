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
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	silenceWindowFinalizerName = "alerts.axonops.com/silence-window-finalizer"
)

// AxonOpsSilenceWindowReconciler reconciles a AxonOpsSilenceWindow object
type AxonOpsSilenceWindowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopssilencewindows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopssilencewindows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopssilencewindows/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsSilenceWindow
func (r *AxonOpsSilenceWindowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.silencewindow", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopssilencewindow", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopssilencewindow", resultStr).Inc()
	}()

	silence := &alertsv1alpha1.AxonOpsSilenceWindow{}
	if err := r.Get(ctx, req.NamespacedName, silence); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsSilenceWindow", "silence", req.NamespacedName)

	if silence.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, silence)
	}

	if !controllerutil.ContainsFinalizer(silence, silenceWindowFinalizerName) {
		controllerutil.AddFinalizer(silence, silenceWindowFinalizerName)
		if err := r.Update(ctx, silence); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := ResolveAPIClient(ctx, r.Client, silence.Namespace, silence.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		meta.SetStatusCondition(&silence.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: silence.Generation,
			Reason:             "FailedToResolveConnection",
			Message:            fmt.Sprintf("Failed to resolve connection: %v", err),
		})
		silence.Status.ObservedGeneration = silence.Generation
		if err := r.Status().Update(ctx, silence); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(silence.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		silence.Status.ObservedGeneration == silence.Generation &&
		silence.Status.SyncedSilenceID != "" {
		log.Info("Silence already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	active := true
	if silence.Spec.Active != nil {
		active = *silence.Spec.Active
	}

	// If updating, delete the old silence first
	if silence.Status.SyncedSilenceID != "" {
		log.Info("Deleting existing silence before recreate", "silenceID", silence.Status.SyncedSilenceID)
		if err := apiClient.DeleteSilenceWindow(ctx, silence.Spec.ClusterType, silence.Spec.ClusterName, silence.Status.SyncedSilenceID); err != nil {
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	}

	syncedID := ""
	if active {
		// Build and create the silence
		dcs := silence.Spec.Datacenters
		if dcs == nil {
			dcs = []string{}
		}

		payload := axonops.SilenceWindow{
			ID:          uuid.New().String(),
			Active:      true,
			CronExpr:    silence.Spec.CronExpression,
			IsRecurring: silence.Spec.Recurring,
			Duration:    silence.Spec.Duration,
			DCs:         dcs,
		}

		if err := apiClient.CreateSilenceWindow(ctx, silence.Spec.ClusterType, silence.Spec.ClusterName, payload); err != nil {
			log.Error(err, "Failed to create silence window")
			meta.SetStatusCondition(&silence.Status.Conditions, metav1.Condition{
				Type:               "Failed",
				Status:             metav1.ConditionTrue,
				ObservedGeneration: silence.Generation,
				Reason:             "SyncFailed",
				Message:            fmt.Sprintf("Failed to sync with AxonOps: %v", err),
			})
			silence.Status.ObservedGeneration = silence.Generation
			if err := r.Status().Update(ctx, silence); err != nil {
				log.Error(err, "Failed to update status")
			}
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, nil
		}
		syncedID = payload.ID
	}

	// Re-fetch and update status
	syncedGeneration := silence.Generation
	if err := r.Get(ctx, req.NamespacedName, silence); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	silence.Status.SyncedSilenceID = syncedID
	now := metav1.Now()
	silence.Status.LastSyncTime = &now
	silence.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&silence.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Silence window synced with AxonOps",
	})
	meta.RemoveStatusCondition(&silence.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, silence); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced silence window", "silenceID", syncedID)
	return ctrl.Result{}, nil
}

func (r *AxonOpsSilenceWindowReconciler) handleDeletion(ctx context.Context, silence *alertsv1alpha1.AxonOpsSilenceWindow) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(silence, silenceWindowFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting silence window from AxonOps", "silenceID", silence.Status.SyncedSilenceID)

	apiClient, err := ResolveAPIClient(ctx, r.Client, silence.Namespace, silence.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if silence.Status.SyncedSilenceID != "" {
		if err := apiClient.DeleteSilenceWindow(ctx, silence.Spec.ClusterType, silence.Spec.ClusterName, silence.Status.SyncedSilenceID); err != nil {
			log.Error(err, "Failed to delete silence window", "silenceID", silence.Status.SyncedSilenceID)
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted silence window from AxonOps")
		}
	}

	controllerutil.RemoveFinalizer(silence, silenceWindowFinalizerName)
	if err := r.Update(ctx, silence); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsSilenceWindowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsSilenceWindow{}).
		Named("alerts-axonopssilencewindow").
		Complete(r)
}
