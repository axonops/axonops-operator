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
	"os"
	"time"

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
	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
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
func (r *AxonOpsMetricAlertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsMetricAlert CR
	alert := &alertsv1alpha1.AxonOpsMetricAlert{}
	if err := r.Get(ctx, req.NamespacedName, alert); err != nil {
		// Resource not found, not an error
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsMetricAlert", "alert", req.NamespacedName)

	// Handle deletion
	if alert.ObjectMeta.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, alert)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(alert, finalizerName) {
		controllerutil.AddFinalizer(alert, finalizerName)
		if err := r.Update(ctx, alert); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Resolve AxonOps API client from connection or environment
	apiClient, err := r.resolveAPIClient(ctx, alert)
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
		if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Update status with synced alert ID and timestamps
	alert.Status.SyncedAlertID = result.ID
	now := metav1.Now()
	alert.Status.LastSyncTime = &now
	alert.Status.ObservedGeneration = alert.Generation

	// Set Ready condition
	meta.SetStatusCondition(&alert.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: alert.Generation,
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

	log.Info("Deleting alert rule from AxonOps", "alertID", alert.Status.SyncedAlertID)

	// Only delete if we have a synced alert ID
	if alert.Status.SyncedAlertID != "" {
		// Resolve AxonOps API client for deletion
		apiClient, err := r.resolveAPIClient(ctx, alert)
		if err != nil {
			log.Error(err, "Failed to resolve API client for deletion")
			// If connection is not found, we can't delete remotely, but proceed with finalizer removal
			// This might happen if the connection was deleted before the alert
		} else {
			if err := apiClient.DeleteMetricAlertRule(ctx, alert.Spec.ClusterType, alert.Spec.ClusterName, alert.Status.SyncedAlertID); err != nil {
				log.Error(err, "Failed to delete alert rule from AxonOps")
				// Check if error is retryable
				if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				// Don't retry on client errors
			}
		}
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
	rule.Expr = fmt.Sprintf("%s %s %g", alert.Spec.Metric, alert.Spec.Operator, alert.Spec.WarningValue)

	// Add annotations if present
	if alert.Spec.Annotations != nil {
		rule.Annotations = axonops.MetricAlertAnnotations{
			Summary:     alert.Spec.Annotations.Summary,
			Description: alert.Spec.Annotations.Description,
			WidgetUrl:   alert.Spec.Annotations.WidgetURL,
		}
	}

	// Add integrations if present
	if len(alert.Spec.Integrations) > 0 {
		for _, integ := range alert.Spec.Integrations {
			rule.Integrations = axonops.MetricAlertIntegrations{
				Type:            integ.Type,
				Routing:         integ.Routing,
				OverrideInfo:    integ.OverrideInfo,
				OverrideWarning: integ.OverrideWarning,
				OverrideError:   integ.OverrideError,
			}
		}
	}

	// Add filters if present
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
		if len(filters.Scope) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "scope", Value: filters.Scope})
		}
		if len(filters.Keyspace) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "keyspace", Value: filters.Keyspace})
		}
		if len(filters.Percentile) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "percentile", Value: filters.Percentile})
		}
		if len(filters.Consistency) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "consistency", Value: filters.Consistency})
		}
		if len(filters.Topic) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "topic", Value: filters.Topic})
		}
		if len(filters.GroupID) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "group_id", Value: filters.GroupID})
		}
		if len(filters.GroupBy) > 0 {
			rule.Filters = append(rule.Filters, axonops.MetricAlertFilter{Name: "group_by", Value: filters.GroupBy})
		}
	}

	return rule
}

// resolveAPIClient resolves the AxonOps API client from either a referenced AxonOpsConnection
// or from environment variables (fallback).
func (r *AxonOpsMetricAlertReconciler) resolveAPIClient(ctx context.Context, alert *alertsv1alpha1.AxonOpsMetricAlert) (*axonops.Client, error) {
	log := logf.FromContext(ctx)

	// If connectionRef is specified, resolve from the connection resource
	if alert.Spec.ConnectionRef != "" {
		conn := &corev1alpha1.AxonOpsConnection{}
		connKey := types.NamespacedName{Namespace: alert.Namespace, Name: alert.Spec.ConnectionRef}
		if err := r.Get(ctx, connKey, conn); err != nil {
			return nil, fmt.Errorf("failed to get AxonOpsConnection: %w", err)
		}

		// Read the API key from the referenced secret
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: alert.Namespace, Name: conn.Spec.APIKeyRef.Name}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			return nil, fmt.Errorf("failed to get secret %s: %w", secretKey, err)
		}

		// Extract the API key from the secret
		keyName := conn.Spec.APIKeyRef.Key
		if keyName == "" {
			keyName = "api_key"
		}
		apiKey, ok := secret.Data[keyName]
		if !ok {
			return nil, fmt.Errorf("secret %s does not have key %q", secretKey, keyName)
		}

		// Build client from connection spec
		protocol := conn.Spec.Protocol
		if protocol == "" {
			protocol = "https"
		}
		tokenType := conn.Spec.TokenType
		if tokenType == "" {
			tokenType = "Bearer"
		}

		// Construct host URL
		fullHost := buildHostURL(conn.Spec.Host, conn.Spec.OrgID, conn.Spec.UseSAML)

		client, err := axonops.NewClient(fullHost, "", conn.Spec.OrgID, string(apiKey), tokenType, conn.Spec.TLSSkipVerify)
		if err != nil {
			return nil, fmt.Errorf("failed to create AxonOps client from connection: %w", err)
		}
		log.Info("Resolved API client from AxonOpsConnection", "connection", connKey)
		return client, nil
	}

	// Fallback to environment variables
	log.Info("No connectionRef specified, falling back to environment variables")
	host := os.Getenv("AXONOPS_HOST")
	protocol := os.Getenv("AXONOPS_PROTOCOL")
	orgID := os.Getenv("AXONOPS_ORG_ID")
	apiKey := os.Getenv("AXONOPS_API_KEY")
	tokenType := os.Getenv("AXONOPS_TOKEN_TYPE")
	tlsSkipVerify := os.Getenv("AXONOPS_TLS_SKIP_VERIFY") == "true"

	if apiKey == "" {
		return nil, fmt.Errorf("no connectionRef provided and AXONOPS_API_KEY environment variable not set")
	}
	if orgID == "" {
		return nil, fmt.Errorf("no connectionRef provided and AXONOPS_ORG_ID environment variable not set")
	}

	client, err := axonops.NewClient(host, protocol, orgID, apiKey, tokenType, tlsSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("failed to create AxonOps client from environment: %w", err)
	}
	return client, nil
}

// buildHostURL constructs the AxonOps API host URL based on the connection settings
// This mirrors the Terraform provider's URL construction logic
func buildHostURL(customHost, orgID string, useSAML bool) string {
	var host string
	if customHost == "" {
		// No custom host - use SaaS defaults
		if useSAML {
			host = fmt.Sprintf("%s.axonops.cloud/dashboard", orgID)
		} else {
			host = fmt.Sprintf("dash.axonops.cloud/%s", orgID)
		}
	} else {
		// Custom host provided
		if useSAML {
			host = fmt.Sprintf("%s/dashboard", customHost)
		} else {
			host = fmt.Sprintf("%s/%s", customHost, orgID)
		}
	}

	// Return URL with https protocol (protocol handling is done by axonops.NewClient)
	return fmt.Sprintf("https://%s", host)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsMetricAlertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsMetricAlert{}).
		Named("alerts-axonopsmetricalert").
		Complete(r)
}
