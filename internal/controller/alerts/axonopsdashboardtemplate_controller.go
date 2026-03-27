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
	"encoding/json"
	"errors"
	"fmt"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	dashboardTemplateFinalizer = "alerts.axonops.com/dashboard-template-finalizer"
	configMapRefIndexField     = ".spec.source.configMapRef.name"
	dashboardSizeWarningBytes  = 500 * 1024 // 500KB
)

// AxonOpsDashboardTemplateReconciler reconciles a AxonOpsDashboardTemplate object
type AxonOpsDashboardTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsdashboardtemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsdashboardtemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsdashboardtemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsDashboardTemplate
func (r *AxonOpsDashboardTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.dashboardtemplate", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsdashboardtemplate", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsdashboardtemplate", resultStr).Inc()
	}()

	// Fetch the CR
	dashboard := &alertsv1alpha1.AxonOpsDashboardTemplate{}
	if err := r.Get(ctx, req.NamespacedName, dashboard); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsDashboardTemplate", "dashboard", req.NamespacedName)

	// Handle deletion
	if dashboard.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, dashboard)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(dashboard, dashboardTemplateFinalizer) {
		controllerutil.AddFinalizer(dashboard, dashboardTemplateFinalizer)
		if err := r.Update(ctx, dashboard); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, dashboard.Namespace, dashboard.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, dashboard, &dashboard.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, dashboard, ReasonConnectionError, common.SafeConditionMsg("Failed to resolve connection", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if already synced and spec unchanged
	readyCond := meta.FindStatusCondition(dashboard.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		dashboard.Status.ObservedGeneration == dashboard.Generation &&
		dashboard.Status.LastSyncTime != nil {
		log.Info("Dashboard template already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// Validate and resolve source
	filters, panels, err := r.resolveSource(ctx, dashboard)
	if err != nil {
		return ctrl.Result{}, nil // condition already set by resolveSource
	}

	// Count panels for status
	var panelList []json.RawMessage
	if panels != nil {
		if err := json.Unmarshal(panels, &panelList); err != nil {
			log.Error(err, "Failed to count panels")
		}
	}
	panelCount := int32(len(panelList))

	// Warn on large dashboard
	totalSize := len(filters) + len(panels)
	if totalSize > dashboardSizeWarningBytes {
		log.Info("Dashboard JSON is large, approaching size limits", "sizeBytes", totalSize)
	}

	// GET existing dashboards from API
	existing, err := apiClient.GetDashboardTemplates(ctx, dashboard.Spec.ClusterType, dashboard.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get dashboard templates from AxonOps")
		r.setFailedCondition(ctx, dashboard, ReasonAPIError, common.SafeConditionMsg("Failed to get dashboards", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build desired dashboard entry
	desired := axonops.DashboardTemplateRaw{
		Name:    dashboard.Spec.DashboardName,
		Filters: filters,
		Panels:  panels,
	}

	// Merge into existing list: replace by name or append
	dashboards := r.mergeDashboard(existing.Dashboards, desired)

	// PUT full list
	payload := axonops.DashboardTemplatePutPayload{
		Type:       dashboard.Spec.ClusterType,
		Dashboards: dashboards,
	}
	if err := apiClient.UpdateDashboardTemplates(ctx, dashboard.Spec.ClusterType, dashboard.Spec.ClusterName, payload); err != nil {
		log.Error(err, "Failed to update dashboard templates")
		r.setFailedCondition(ctx, dashboard, ReasonAPIError, common.SafeConditionMsg("Failed to update dashboards", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Update status
	syncedGeneration := dashboard.Generation
	if err := r.Get(ctx, req.NamespacedName, dashboard); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := metav1.Now()
	dashboard.Status.LastSyncTime = &now
	dashboard.Status.ObservedGeneration = syncedGeneration
	dashboard.Status.PanelCount = panelCount

	meta.SetStatusCondition(&dashboard.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Dashboard template synced with AxonOps",
	})
	meta.RemoveStatusCondition(&dashboard.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, dashboard); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced dashboard template", "dashboard", dashboard.Spec.DashboardName, "panelCount", panelCount)
	return ctrl.Result{}, nil
}

// resolveSource validates and resolves the dashboard source.
// Returns filters and panels as raw JSON. Sets Failed condition on error.
func (r *AxonOpsDashboardTemplateReconciler) resolveSource(ctx context.Context, dashboard *alertsv1alpha1.AxonOpsDashboardTemplate) (json.RawMessage, json.RawMessage, error) {
	log := logf.FromContext(ctx)
	src := dashboard.Spec.Source

	// Validate exactly one source
	if src.Inline == nil && src.ConfigMapRef == nil {
		r.setFailedCondition(ctx, dashboard, "InvalidSource", "Exactly one of spec.source.inline or spec.source.configMapRef must be set")
		return nil, nil, fmt.Errorf("invalid source: neither inline nor configMapRef set")
	}
	if src.Inline != nil && src.ConfigMapRef != nil {
		r.setFailedCondition(ctx, dashboard, "InvalidSource", "Exactly one of spec.source.inline or spec.source.configMapRef must be set, not both")
		return nil, nil, fmt.Errorf("invalid source: both inline and configMapRef set")
	}

	var rawJSON []byte

	if src.Inline != nil {
		rawJSON = src.Inline.Dashboard.Raw
	} else {
		// Read from ConfigMap
		cm := &corev1.ConfigMap{}
		cmKey := types.NamespacedName{Namespace: dashboard.Namespace, Name: src.ConfigMapRef.Name}
		if err := r.Get(ctx, cmKey, cm); err != nil {
			if client.IgnoreNotFound(err) == nil {
				r.setFailedCondition(ctx, dashboard, "ConfigMapNotFound", fmt.Sprintf("ConfigMap %q not found", src.ConfigMapRef.Name))
				return nil, nil, fmt.Errorf("configmap not found: %s", src.ConfigMapRef.Name)
			}
			log.Error(err, "Failed to get ConfigMap")
			r.setFailedCondition(ctx, dashboard, "ConfigMapNotFound", common.SafeConditionMsg("Failed to get ConfigMap", err))
			return nil, nil, err
		}

		data, ok := cm.Data[src.ConfigMapRef.Key]
		if !ok {
			r.setFailedCondition(ctx, dashboard, "ConfigMapKeyNotFound", fmt.Sprintf("Key %q not found in ConfigMap %q", src.ConfigMapRef.Key, src.ConfigMapRef.Name))
			return nil, nil, fmt.Errorf("key %q not found in configmap %q", src.ConfigMapRef.Key, src.ConfigMapRef.Name)
		}
		rawJSON = []byte(data)
	}

	// Parse the JSON to extract filters and panels
	var parsed struct {
		Filters json.RawMessage `json:"filters"`
		Panels  json.RawMessage `json:"panels"`
	}
	if err := json.Unmarshal(rawJSON, &parsed); err != nil {
		r.setFailedCondition(ctx, dashboard, "InvalidDashboardJSON", common.SafeConditionMsg("Failed to parse dashboard JSON", err))
		return nil, nil, fmt.Errorf("invalid dashboard JSON: %w", err)
	}

	return parsed.Filters, parsed.Panels, nil
}

// mergeDashboard merges the desired dashboard into the existing list by name.
// If a dashboard with the same name exists, it is replaced. Otherwise, the new one is appended.
func (r *AxonOpsDashboardTemplateReconciler) mergeDashboard(existing []axonops.DashboardTemplateRaw, desired axonops.DashboardTemplateRaw) []axonops.DashboardTemplateRaw {
	result := make([]axonops.DashboardTemplateRaw, 0, len(existing)+1)
	found := false
	for _, d := range existing {
		if d.Name == desired.Name {
			// Preserve UUID from existing dashboard
			desired.UUID = d.UUID
			result = append(result, desired)
			found = true
		} else {
			result = append(result, d)
		}
	}
	if !found {
		result = append(result, desired)
	}
	return result
}

// handleDeletion removes the dashboard from the remote list and removes the finalizer
func (r *AxonOpsDashboardTemplateReconciler) handleDeletion(ctx context.Context, dashboard *alertsv1alpha1.AxonOpsDashboardTemplate) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(dashboard, dashboardTemplateFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting dashboard template from AxonOps", "dashboard", dashboard.Spec.DashboardName)

	// Resolve API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, dashboard.Namespace, dashboard.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// GET existing dashboards
	existing, err := apiClient.GetDashboardTemplates(ctx, dashboard.Spec.ClusterType, dashboard.Spec.ClusterName)
	if err != nil {
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
			log.Info("Dashboard templates not found on remote, proceeding with finalizer removal")
		} else {
			log.Error(err, "Failed to get dashboards for deletion — will retry")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else {
		// Remove the dashboard from the list
		filtered := make([]axonops.DashboardTemplateRaw, 0, len(existing.Dashboards))
		found := false
		for _, d := range existing.Dashboards {
			if d.Name == dashboard.Spec.DashboardName {
				found = true
			} else {
				filtered = append(filtered, d)
			}
		}

		if found {
			payload := axonops.DashboardTemplatePutPayload{
				Type:       dashboard.Spec.ClusterType,
				Dashboards: filtered,
			}
			if err := apiClient.UpdateDashboardTemplates(ctx, dashboard.Spec.ClusterType, dashboard.Spec.ClusterName, payload); err != nil {
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				log.Error(err, "Failed to remove dashboard from AxonOps")
			} else {
				log.Info("Successfully removed dashboard from AxonOps")
			}
		} else {
			log.Info("Dashboard not found in remote list, skipping PUT")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(dashboard, dashboardTemplateFinalizer)
	if err := r.Update(ctx, dashboard); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// setFailedCondition sets a failed condition on the dashboard template CR
func (r *AxonOpsDashboardTemplateReconciler) setFailedCondition(ctx context.Context, dashboard *alertsv1alpha1.AxonOpsDashboardTemplate, reason, message string) {
	meta.SetStatusCondition(&dashboard.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: dashboard.Generation,
		Reason:             reason,
		Message:            message,
	})
	dashboard.Status.ObservedGeneration = dashboard.Generation
	if err := r.Status().Update(ctx, dashboard); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// findDashboardsForConfigMap maps a ConfigMap change to the dashboard CRs that reference it
func (r *AxonOpsDashboardTemplateReconciler) findDashboardsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	log := logf.FromContext(ctx)
	dashboardList := &alertsv1alpha1.AxonOpsDashboardTemplateList{}
	if err := r.List(ctx, dashboardList, client.InNamespace(obj.GetNamespace()), client.MatchingFields{configMapRefIndexField: obj.GetName()}); err != nil {
		log.Error(err, "Failed to list dashboard templates for ConfigMap", "configMap", obj.GetName())
		return nil
	}

	requests := make([]reconcile.Request, 0, len(dashboardList.Items))
	for _, d := range dashboardList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      d.Name,
				Namespace: d.Namespace,
			},
		})
	}
	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsDashboardTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Add field indexer for ConfigMap reference lookups
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &alertsv1alpha1.AxonOpsDashboardTemplate{}, configMapRefIndexField, func(obj client.Object) []string {
		dashboard := obj.(*alertsv1alpha1.AxonOpsDashboardTemplate)
		if dashboard.Spec.Source.ConfigMapRef == nil {
			return nil
		}
		return []string{dashboard.Spec.Source.ConfigMapRef.Name}
	}); err != nil {
		return fmt.Errorf("failed to set up field indexer: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsDashboardTemplate{}).
		Watches(&corev1.ConfigMap{}, handler.EnqueueRequestsFromMapFunc(r.findDashboardsForConfigMap)).
		Named("alerts-axonopsdashboardtemplate").
		Complete(r)
}
