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
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

const (
	logFinalizerName = "alerts.axonops.com/log-alert-finalizer"
	logCondTypeReady = "Ready"
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
func (r *AxonOpsLogAlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsLogAlert CR
	alert := &alertsv1alpha1.AxonOpsLogAlert{}
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		// Resource not found, not an error
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsLogAlert", "alert", req.NamespacedName)

	// Handle deletion
	if alert.ObjectMeta.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, alert)
	}

	// Add finalizer if not present — return immediately so the watch triggers a fresh
	// reconcile with an up-to-date ResourceVersion, avoiding status update conflicts.
	if !controllerutil.ContainsFinalizer(alert, logFinalizerName) {
		controllerutil.AddFinalizer(alert, logFinalizerName)
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
	readyCond := meta.FindStatusCondition(alert.Status.Conditions, logCondTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		alert.Status.ObservedGeneration == alert.Generation &&
		alert.Status.SyncedAlertID != "" {
		log.Info("Alert already synced and spec unchanged, skipping API call",
			"syncedID", alert.Status.SyncedAlertID, "generation", alert.Generation)
		return ctrl.Result{}, nil
	}
	log.Info("Alert needs sync - checking conditions",
		"readyCondExists", readyCond != nil,
		"readyCondTrue", readyCond != nil && readyCond.Status == metav1.ConditionTrue,
		"genMatch", alert.Status.ObservedGeneration == alert.Generation,
		"hasSyncedID", alert.Status.SyncedAlertID != "",
		"observedGen", alert.Status.ObservedGeneration,
		"currentGen", alert.Generation)

	// Set Progressing condition
	meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: alert.Generation,
		Reason:             "Syncing",
		Message:            "Syncing alert rule with AxonOps",
	})

	// Build MetricAlertRule from spec (reuse same API payload structure)
	rule := r.buildLogAlertRule(alert)
	log.Info("About to sync log alert with AxonOps", "alertName", alert.Spec.Name, "ruleID", rule.ID, "expr", rule.Expr)

	// Create or update the alert in AxonOps
	result, err := apiClient.CreateOrUpdateMetricAlertRule(ctx, alert.Spec.ClusterType, alert.Spec.ClusterName, rule)
	if err == nil {
		log.Info("Successfully synced log alert, got ID from API", "returnedID", result.ID)
	}
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
		if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Re-fetch the CR to get the latest ResourceVersion before updating status.
	// Without this, a conflict error would cause a requeue where SyncedAlertID is
	// still empty, triggering yet another API create call and producing duplicates.
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
		Type:               logCondTypeReady,
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

	if !controllerutil.ContainsFinalizer(alert, logFinalizerName) {
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
			if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
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
	controllerutil.RemoveFinalizer(alert, logFinalizerName)
	if err := r.Update(ctx, alert); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted alert rule")
	return ctrl.Result{}, nil
}

// buildLogAlertRule constructs a MetricAlertRule from an AxonOpsLogAlert CR
// Log alerts use the same API payload structure but with events{} expression
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

	// Build events{} expression for log alerts
	rule.Expr = buildLogEventsExpr(alert.Spec.Content, alert.Spec.Level, alert.Spec.Source, alert.Spec.LogType)

	// Add annotations if present
	if alert.Spec.Annotations != nil {
		rule.Annotations = axonops.MetricAlertAnnotations{
			Summary:     alert.Spec.Annotations.Summary,
			Description: alert.Spec.Annotations.Description,
			WidgetUrl:   alert.Spec.Annotations.WidgetURL,
		}
	}

	// Note: Integrations are intentionally omitted from API payload.
	// The AxonOps API expects a different structure for integrations
	// that doesn't align with the current CR spec.
	// This can be enhanced in the future once the API contract is clarified.

	return rule
}

// buildLogEventsExpr builds an events{...} expression for log alert rules
// Converts comma-separated values to pipe-separated within event fields
func buildLogEventsExpr(content, level, source, logType string) string {
	var parts []string

	if content != "" {
		escaped := strings.ReplaceAll(content, `"`, `\"`)
		parts = append(parts, fmt.Sprintf(`message="%s"`, escaped))
	}

	if level != "" {
		// Convert comma-separated to pipe-separated
		levelExpr := strings.ReplaceAll(level, ",", "|")
		parts = append(parts, fmt.Sprintf(`level="%s"`, levelExpr))
	}

	if source != "" {
		// Convert comma-separated to pipe-separated
		sourceExpr := strings.ReplaceAll(source, ",", "|")
		parts = append(parts, fmt.Sprintf(`source="%s"`, sourceExpr))
	}

	if logType != "" {
		// Convert comma-separated to pipe-separated
		typeExpr := strings.ReplaceAll(logType, ",", "|")
		parts = append(parts, fmt.Sprintf(`type="%s"`, typeExpr))
	}

	return fmt.Sprintf("events{%s}", strings.Join(parts, ","))
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsLogAlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsLogAlert{}).
		Named("alerts-axonopslogalert").
		Complete(r)
}
