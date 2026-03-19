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
	finalizerName = "alerts.axonops.com/metric-alert-finalizer"
	condTypeReady = "Ready"
)

// AxonOpsMetricAlertReconciler reconciles a AxonOpsMetricAlert object
type AxonOpsMetricAlertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsmetricalerts/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsMetricAlert
func (r *AxonOpsMetricAlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.metricalert", trace.WithAttributes())
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	start := time.Now()
	defer func() {
		resultStr := "success"
		if err != nil {
			resultStr = "error"
			axonopsmetrics.ReconcileErrorsTotal.WithLabelValues(axonopsmetrics.ClassifyError(err)).Inc()
		}
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsmetricalert", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsmetricalert", resultStr).Inc()
	}()

	// Fetch the AxonOpsMetricAlert CR
	alert := &alertsv1alpha1.AxonOpsMetricAlert{}
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		// Resource not found, not an error
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsMetricAlert", "alert", req.NamespacedName)

	// Handle deletion
	if alert.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, alert)
	}

	// Add finalizer if not present — return immediately so the watch triggers a fresh
	// reconcile with an up-to-date ResourceVersion, avoiding status update conflicts.
	if !controllerutil.ContainsFinalizer(alert, finalizerName) {
		controllerutil.AddFinalizer(alert, finalizerName)
		if err := r.Update(ctx, alert); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client from connection or environment
	apiClient, err := ResolveAPIClient(ctx, r.Client, alert.Namespace, alert.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: alert.Generation,
			Reason:             "FailedToResolveConnection",
			Message:            fmt.Sprintf("Failed to resolve connection: %v", err),
		})
		alert.Status.ObservedGeneration = alert.Generation
		if err := r.Status().Update(ctx, alert); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if alert is already synced with observed generation matching
	// This avoids unnecessary API calls on every reconciliation
	readyCond := meta.FindStatusCondition(alert.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		alert.Status.ObservedGeneration == alert.Generation &&
		alert.Status.SyncedAlertID != "" {
		log.Info("Alert already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// Set Progressing condition
	meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: alert.Generation,
		Reason:             "Syncing",
		Message:            "Syncing alert rule with AxonOps",
	})

	// Resolve dashboard panel to get correlationId
	correlationID, err := apiClient.ResolveDashboardPanel(ctx, alert.Spec.ClusterType, alert.Spec.ClusterName, alert.Spec.Dashboard, alert.Spec.Chart)
	if err != nil {
		log.Error(err, "Failed to resolve dashboard panel", "dashboard", alert.Spec.Dashboard, "chart", alert.Spec.Chart)
		meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: alert.Generation,
			Reason:             "DashboardResolutionFailed",
			Message:            fmt.Sprintf("Failed to resolve dashboard: %v", err),
		})
		alert.Status.ObservedGeneration = alert.Generation
		if err := r.Status().Update(ctx, alert); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	alert.Status.CorrelationID = correlationID

	// Build MetricAlertRule from spec
	rule := r.buildMetricAlertRule(alert)

	// Create or update the alert in AxonOps
	result, err := apiClient.CreateOrUpdateMetricAlertRule(ctx, alert.Spec.ClusterType, alert.Spec.ClusterName, rule)
	if err != nil {
		log.Error(err, "Failed to create/update alert rule")
		meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: alert.Generation,
			Reason:             "SyncFailed",
			Message:            fmt.Sprintf("Failed to sync with AxonOps: %v", err),
		})
		alert.Status.ObservedGeneration = alert.Generation
		if err := r.Status().Update(ctx, alert); err != nil {
			log.Error(err, "Failed to update status")
		}

		// Check if error is retryable
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Re-fetch the CR to get the latest ResourceVersion before updating status.
	// Without this, a conflict error would cause a requeue where SyncedAlertID is
	// still empty, triggering yet another API create call and producing duplicates.
	syncedID := result.ID
	correlationID = alert.Status.CorrelationID
	syncedGeneration := alert.Generation
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	alert.Status.SyncedAlertID = syncedID
	alert.Status.CorrelationID = correlationID
	now := metav1.Now()
	alert.Status.LastSyncTime = &now
	alert.Status.ObservedGeneration = syncedGeneration

	// Set Ready condition
	meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Alert rule synced with AxonOps",
	})

	// Remove Progressing and Failed conditions
	meta.RemoveStatusCondition(&alert.Status.Conditions, "Progressing")
	meta.RemoveStatusCondition(&alert.Status.Conditions, "Failed")

	// Update the CR status
	if err := r.Status().Update(ctx, alert); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced alert rule", "alertID", result.ID)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsMetricAlertReconciler) handleDeletion(ctx context.Context, alert *alertsv1alpha1.AxonOpsMetricAlert) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(alert, finalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting alert rule from AxonOps",
		"alertID", alert.Status.SyncedAlertID,
		"clusterType", alert.Spec.ClusterType,
		"clusterName", alert.Spec.ClusterName)

	// Resolve AxonOps API client for deletion
	apiClient, err := ResolveAPIClient(ctx, r.Client, alert.Namespace, alert.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Delete the alert using the unique ID stored in status
	if alert.Status.SyncedAlertID != "" {
		log.Info("Deleting alert rule from AxonOps API", "alertID", alert.Status.SyncedAlertID)
		if err := apiClient.DeleteMetricAlertRule(ctx, alert.Spec.ClusterType, alert.Spec.ClusterName, alert.Status.SyncedAlertID); err != nil {
			log.Error(err, "Failed to delete alert rule from AxonOps", "alertID", alert.Status.SyncedAlertID)
			// Check for retryable API errors
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			// For non-retryable errors (like 404), proceed to remove the finalizer
		} else {
			log.Info("Successfully deleted alert rule from AxonOps API", "alertID", alert.Status.SyncedAlertID)
		}
	} else {
		log.Info("SyncedAlertID is empty, skipping API deletion (resource may not have been created)")
	}

	// Remove the finalizer
	controllerutil.RemoveFinalizer(alert, finalizerName)
	if err := r.Update(ctx, alert); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted alert rule")
	return ctrl.Result{}, nil
}

// buildMetricAlertRule constructs a MetricAlertRule from an AxonOpsMetricAlert CR
func (r *AxonOpsMetricAlertReconciler) buildMetricAlertRule(alert *alertsv1alpha1.AxonOpsMetricAlert) axonops.MetricAlertRule {
	rule := axonops.MetricAlertRule{
		Alert:         alert.Spec.Name,
		For:           alert.Spec.Duration,
		Operator:      alert.Spec.Operator,
		WarningValue:  alert.Spec.WarningValue,
		CriticalValue: alert.Spec.CriticalValue,
		WidgetTitle:   alert.Spec.Chart,
		CorrelationId: alert.Status.CorrelationID,
	}

	// Set ID if we have one from a previous sync
	if alert.Status.SyncedAlertID != "" {
		rule.ID = alert.Status.SyncedAlertID
	}

	// Build expression: metric op value
	// NOTE: Currently uses WarningValue in the expression. This may need review to ensure
	// CriticalValue is properly handled. The API payload includes both thresholds separately,
	// but the expression only references the warning value. Verify with AxonOps API docs
	// whether this is the intended behavior or if the expression should reference CriticalValue.
	// See: https://github.com/axonops/axonops-operator/issues/1
	rule.Expr = fmt.Sprintf("%s %s %g", alert.Spec.Metric, alert.Spec.Operator, alert.Spec.WarningValue)

	// Add annotations if present
	if alert.Spec.Annotations != nil {
		rule.Annotations = axonops.MetricAlertAnnotations{
			Summary:     alert.Spec.Annotations.Summary,
			Description: alert.Spec.Annotations.Description,
			WidgetUrl:   alert.Spec.Annotations.WidgetURL,
		}
	}

	// Note: Integrations are intentionally omitted from API payload.
	// Alert routing should be configured via the AxonOpsAlertRoute CRD instead,
	// which provides a cleaner separation of concerns and matches Kubernetes patterns.
	// The inline Integrations field is deprecated and may be removed in a future version.

	// Add filters if present (data-driven approach for maintainability)
	if alert.Spec.Filters != nil {
		filters := alert.Spec.Filters
		filterMap := map[string][]string{
			"dc":          filters.DataCenter,
			"rack":        filters.Rack,
			"host_id":     filters.HostID,
			"scope":       filters.Scope,
			"keyspace":    filters.Keyspace,
			"percentile":  filters.Percentile,
			"consistency": filters.Consistency,
			"topic":       filters.Topic,
			"group_id":    filters.GroupID,
			"group_by":    filters.GroupBy,
		}
		for name, values := range filterMap {
			if len(values) > 0 {
				rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: name, Value: values})
			}
		}
	}

	return rule
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsMetricAlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsMetricAlert{}).
		Named("alerts-axonopsmetricalert").
		Complete(r)
}
