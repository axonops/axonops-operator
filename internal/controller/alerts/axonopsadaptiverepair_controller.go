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

package alerts

import (
	"context"
	"errors"
	"reflect"
	"time"

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
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	adaptiveRepairFinalizer = "alerts.axonops.com/adaptive-repair-finalizer"
)

// AxonOpsAdaptiveRepairReconciler reconciles a AxonOpsAdaptiveRepair object
type AxonOpsAdaptiveRepairReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsadaptiverepairs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsadaptiverepairs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsadaptiverepairs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsAdaptiveRepair
func (r *AxonOpsAdaptiveRepairReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	// OpenTelemetry span
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.adaptiverepair",
		trace.WithAttributes())
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Prometheus metrics
	start := time.Now()
	defer func() {
		resultStr := axonopsmetrics.ResultSuccess
		if err != nil {
			resultStr = axonopsmetrics.ResultError
			axonopsmetrics.ReconcileErrorsTotal.WithLabelValues(axonopsmetrics.ClassifyError(err)).Inc()
		}
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsadaptiverepair", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsadaptiverepair", resultStr).Inc()
	}()

	// Fetch the AxonOpsAdaptiveRepair CR
	repair := &alertsv1alpha1.AxonOpsAdaptiveRepair{}
	if err := r.Get(ctx, req.NamespacedName, repair); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsAdaptiveRepair", "adaptiveRepair", req.NamespacedName)

	// Handle deletion
	if repair.DeletionTimestamp != nil {
		return ctrl.Result{}, r.handleDeletion(ctx, repair)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(repair, adaptiveRepairFinalizer) {
		controllerutil.AddFinalizer(repair, adaptiveRepairFinalizer)
		if err := r.Update(ctx, repair); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, repair.Namespace, repair.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, repair, &repair.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, repair, ReasonConnectionError, common.SafeConditionMsg("Failed to resolve connection", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if already synced and spec unchanged
	readyCond := meta.FindStatusCondition(repair.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		repair.Status.ObservedGeneration == repair.Generation &&
		repair.Status.LastSyncTime != nil {
		log.Info("Adaptive repair already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// Get current settings from API
	currentSettings, err := apiClient.GetAdaptiveRepair(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get adaptive repair settings from AxonOps")
		r.setFailedCondition(ctx, repair, ReasonAPIError, common.SafeConditionMsg("Failed to get adaptive repair settings", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build desired settings from spec
	desired := r.buildAdaptiveRepairSettings(repair)

	// Compare desired with current; only POST if different
	if r.settingsEqual(desired, *currentSettings) {
		log.Info("Adaptive repair settings match remote, no update needed")
		// Still update status to mark as synced
		return ctrl.Result{}, r.updateReadyStatus(ctx, req, repair)
	}

	// Update settings via API
	if err := apiClient.UpdateAdaptiveRepair(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName, desired); err != nil {
		log.Error(err, "Failed to update adaptive repair settings")
		r.setFailedCondition(ctx, repair, ReasonAPIError, common.SafeConditionMsg("Failed to update adaptive repair settings", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	log.Info("Successfully synced adaptive repair settings")
	return ctrl.Result{}, r.updateReadyStatus(ctx, req, repair)
}

// handleDeletion handles cleanup when the CR is being deleted.
// Adaptive repair is a cluster-level singleton -- it cannot be "deleted" via the API.
// The finalizer simply removes itself without making any API calls.
func (r *AxonOpsAdaptiveRepairReconciler) handleDeletion(ctx context.Context, repair *alertsv1alpha1.AxonOpsAdaptiveRepair) error {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(repair, adaptiveRepairFinalizer) {
		return nil
	}

	log.Info("Removing finalizer for adaptive repair CR, no API cleanup needed (singleton resource)")

	controllerutil.RemoveFinalizer(repair, adaptiveRepairFinalizer)
	if err := r.Update(ctx, repair); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return err
	}

	return nil
}

// buildAdaptiveRepairSettings converts the CRD spec to the API payload struct
func (r *AxonOpsAdaptiveRepairReconciler) buildAdaptiveRepairSettings(repair *alertsv1alpha1.AxonOpsAdaptiveRepair) axonops.AdaptiveRepairSettings {
	settings := axonops.AdaptiveRepairSettings{
		Active:           true,
		FilterTWCSTables: true,
		GcGraceThreshold: 86400,
		TableParallelism: 10,
		SegmentRetries:   3,
		SegmentTimeout:   "2h",
	}

	if repair.Spec.Active != nil {
		settings.Active = *repair.Spec.Active
	}
	if repair.Spec.GcGraceThreshold != nil {
		settings.GcGraceThreshold = *repair.Spec.GcGraceThreshold
	}
	if repair.Spec.TableParallelism != nil {
		settings.TableParallelism = *repair.Spec.TableParallelism
	}
	if repair.Spec.FilterTWCSTables != nil {
		settings.FilterTWCSTables = *repair.Spec.FilterTWCSTables
	}
	if repair.Spec.SegmentRetries != nil {
		settings.SegmentRetries = *repair.Spec.SegmentRetries
	}
	if repair.Spec.SegmentTargetSizeMB != nil {
		settings.SegmentTargetSizeMB = *repair.Spec.SegmentTargetSizeMB
	}
	if repair.Spec.SegmentTimeout != "" {
		settings.SegmentTimeout = repair.Spec.SegmentTimeout
	}
	if repair.Spec.MaxSegmentsPerTable != nil {
		settings.MaxSegmentsPerTable = *repair.Spec.MaxSegmentsPerTable
	}

	// Map excludedTables -> BlacklistedTables; nil treated as empty
	if repair.Spec.ExcludedTables != nil {
		settings.BlacklistedTables = repair.Spec.ExcludedTables
	} else {
		settings.BlacklistedTables = []string{}
	}

	return settings
}

// settingsEqual compares two AdaptiveRepairSettings structs for equality.
// Nil BlacklistedTables is treated as empty slice.
func (r *AxonOpsAdaptiveRepairReconciler) settingsEqual(desired, current axonops.AdaptiveRepairSettings) bool {
	// Normalize nil slices to empty for comparison
	if desired.BlacklistedTables == nil {
		desired.BlacklistedTables = []string{}
	}
	if current.BlacklistedTables == nil {
		current.BlacklistedTables = []string{}
	}
	return reflect.DeepEqual(desired, current)
}

// updateReadyStatus re-fetches the CR and sets the Ready status
func (r *AxonOpsAdaptiveRepairReconciler) updateReadyStatus(ctx context.Context, req ctrl.Request, repair *alertsv1alpha1.AxonOpsAdaptiveRepair) error {
	syncedGeneration := repair.Generation
	if err := r.Get(ctx, req.NamespacedName, repair); err != nil {
		return client.IgnoreNotFound(err)
	}

	now := metav1.Now()
	repair.Status.LastSyncTime = &now
	repair.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Adaptive repair settings synced with AxonOps",
	})
	meta.RemoveStatusCondition(&repair.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, repair); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
		return err
	}

	return nil
}

// setFailedCondition sets a failed condition on the adaptive repair CR
func (r *AxonOpsAdaptiveRepairReconciler) setFailedCondition(ctx context.Context, repair *alertsv1alpha1.AxonOpsAdaptiveRepair, reason, message string) {
	meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: repair.Generation,
		Reason:             reason,
		Message:            message,
	})
	repair.Status.ObservedGeneration = repair.Generation
	if err := r.Status().Update(ctx, repair); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsAdaptiveRepairReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsAdaptiveRepair{}).
		Named("alerts-axonopsadaptiverepair").
		Complete(r)
}
