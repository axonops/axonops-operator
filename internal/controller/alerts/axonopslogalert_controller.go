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
	logAlertFinalizerName = "alerts.axonops.com/log-alert-finalizer"
	logAlertCondTypeReady = "Ready"
)

// AxonOpsLogAlertReconciler reconciles a AxonOpsLogAlert object
type AxonOpsLogAlertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogalerts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogalerts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogalerts/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsLogAlert
func (r *AxonOpsLogAlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.logalert", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopslogalert", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopslogalert", resultStr).Inc()
	}()

	// Fetch the AxonOpsLogAlert CR
	alert := &alertsv1alpha1.AxonOpsLogAlert{}
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		// Resource not found, not an error
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsLogAlert", "alert", req.NamespacedName)

	// Handle deletion
	if alert.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, alert)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(alert, logAlertFinalizerName) {
		controllerutil.AddFinalizer(alert, logAlertFinalizerName)
		if err := r.Update(ctx, alert); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client from connection
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
	readyCond := meta.FindStatusCondition(alert.Status.Conditions, logAlertCondTypeReady)
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

	// Build LogAlertRule from spec (log alerts use same API as metric alerts)
	rule := r.buildLogAlertRule(alert)

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

	// Re-fetch the CR to get the latest ResourceVersion before updating status
	syncedID := result.ID
	syncedGeneration := alert.Generation
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	alert.Status.SyncedAlertID = syncedID
	now := metav1.Now()
	alert.Status.LastSyncTime = &now
	alert.Status.ObservedGeneration = syncedGeneration

	// Set Ready condition
	meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
		Type:               logAlertCondTypeReady,
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
func (r *AxonOpsLogAlertReconciler) handleDeletion(ctx context.Context, alert *alertsv1alpha1.AxonOpsLogAlert) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(alert, logAlertFinalizerName) {
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
	controllerutil.RemoveFinalizer(alert, logAlertFinalizerName)
	if err := r.Update(ctx, alert); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted alert rule")
	return ctrl.Result{}, nil
}

// buildLogAlertRule constructs a MetricAlertRule from an AxonOpsLogAlert CR
// Log alerts use the same API endpoint as metric alerts, but with different expr syntax
func (r *AxonOpsLogAlertReconciler) buildLogAlertRule(alert *alertsv1alpha1.AxonOpsLogAlert) axonops.MetricAlertRule {
	rule := axonops.MetricAlertRule{
		Alert:         alert.Spec.Name,
		For:           alert.Spec.Duration,
		Operator:      alert.Spec.Operator,
		WarningValue:  alert.Spec.WarningValue,
		CriticalValue: alert.Spec.CriticalValue,
	}

	// Set ID if we have one from a previous sync
	if alert.Status.SyncedAlertID != "" {
		rule.ID = alert.Status.SyncedAlertID
	}

	// Build log events expression
	rule.Expr = buildLogEventsExpr(alert.Spec.Content, alert.Spec.Level, alert.Spec.Source, alert.Spec.LogType)

	// Add annotations if present
	if alert.Spec.Annotations != nil {
		rule.Annotations = axonops.MetricAlertAnnotations{
			Summary:     alert.Spec.Annotations.Summary,
			Description: alert.Spec.Annotations.Description,
		}
	}

	// Add filters if present (data center, rack, host)
	if alert.Spec.Filters != nil {
		filters := alert.Spec.Filters
		if len(filters.DataCenter) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "dc", Value: filters.DataCenter})
		}
		if len(filters.Rack) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "rack", Value: filters.Rack})
		}
		if len(filters.HostID) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "host_id", Value: filters.HostID})
		}
	}

	// Note: Integrations are intentionally omitted from API payload.
	// Alert routing should be configured via the AxonOpsAlertRoute CRD instead,
	// which provides a cleaner separation of concerns and matches Kubernetes patterns.

	return rule
}

// buildLogEventsExpr constructs a log events expression from log alert filters
// Format: events{message="content",level="error|warning",source="path",type="logType"}
func buildLogEventsExpr(content, level, source, logType string) string {
	var parts []string

	if content != "" {
		// Escape backslashes first, then quotes to properly handle special characters
		escaped := strings.ReplaceAll(content, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		parts = append(parts, fmt.Sprintf(`message="%s"`, escaped))
	}

	if level != "" {
		parts = append(parts, fmt.Sprintf(`level="%s"`, commaSeparatedToPipe(level)))
	}

	if source != "" {
		parts = append(parts, fmt.Sprintf(`source="%s"`, commaSeparatedToPipe(source)))
	}

	if logType != "" {
		parts = append(parts, fmt.Sprintf(`type="%s"`, commaSeparatedToPipe(logType)))
	}

	return fmt.Sprintf("events{%s}", strings.Join(parts, ","))
}

// commaSeparatedToPipe converts comma-separated values to pipe-separated format
// for use in log event expressions (e.g., "error,warning" -> "error|warning")
func commaSeparatedToPipe(s string) string {
	return strings.ReplaceAll(s, ",", "|")
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsLogAlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsLogAlert{}).
		Named("alerts-axonopslogalert").
		Complete(r)
}
