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
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	alertRouteFinalizerName = "alerts.axonops.com/alert-route-finalizer"
)

// Route type mapping: CRD name -> API URL-encoded name
var routeTypeMap = map[string]string{
	"global":         "Global",
	"metrics":        "Metrics",
	"backups":        "Backups",
	"servicechecks":  "Service%20Checks",
	"nodes":          "Nodes",
	"commands":       "Commands",
	"repairs":        "Repairs",
	"rollingrestart": "Rolling%20Restart",
}

// AxonOpsAlertRouteReconciler reconciles a AxonOpsAlertRoute object
type AxonOpsAlertRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertroutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertroutes/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AxonOpsAlertRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.alertroute", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsalertroute", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsalertroute", resultStr).Inc()
	}()

	// Fetch the AxonOpsAlertRoute instance
	route := &alertsv1alpha1.AxonOpsAlertRoute{}
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if route.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, route)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(route, alertRouteFinalizerName) {
		controllerutil.AddFinalizer(route, alertRouteFinalizerName)
		if err := r.Update(ctx, route); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		// Return immediately after adding finalizer to avoid conflicts
		return ctrl.Result{}, nil
	}

	// Check idempotency: skip if already synced and generation unchanged
	readyCond := meta.FindStatusCondition(route.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		route.Status.ObservedGeneration == route.Generation &&
		route.Status.IntegrationID != "" {
		log.Info("Route already synced, skipping reconciliation",
			"integrationID", route.Status.IntegrationID, "generation", route.Generation)
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling AxonOpsAlertRoute",
		"clusterName", route.Spec.ClusterName,
		"type", route.Spec.Type,
		"severity", route.Spec.Severity,
		"integration", route.Spec.IntegrationName)

	// Resolve API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, route.Namespace, route.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, route, &route.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve API client")
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "ConnectionError",
			Message:            common.SafeConditionMsg("Failed to resolve AxonOps connection", err),
			ObservedGeneration: route.Generation,
		})
		if err := r.Status().Update(ctx, route); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Get integrations to find the integration ID
	integrations, err := apiClient.GetIntegrations(ctx, route.Spec.ClusterType, route.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get integrations")
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "APIError",
			Message:            common.SafeConditionMsg("Failed to get integrations", err),
			ObservedGeneration: route.Generation,
		})
		if err := r.Status().Update(ctx, route); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Find integration ID by name and type
	integrationID, err := findIntegrationID(integrations, route.Spec.IntegrationName, route.Spec.IntegrationType)
	if err != nil {
		log.Error(err, "Integration not found")
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "IntegrationNotFound",
			Message:            fmt.Sprintf("Integration %s of type %s not found", route.Spec.IntegrationName, route.Spec.IntegrationType),
			ObservedGeneration: route.Generation,
		})
		if err := r.Status().Update(ctx, route); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Get API route type (URL-encoded)
	apiRouteType, err := getAPIRouteType(route.Spec.Type)
	if err != nil {
		log.Error(err, "Invalid route type")
		meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidRouteType",
			Message:            fmt.Sprintf("Unknown route type: %s", route.Spec.Type),
			ObservedGeneration: route.Generation,
		})
		if err := r.Status().Update(ctx, route); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, err
	}

	// Check if route already exists
	routeExists := checkRouteExists(integrations, apiRouteType, route.Spec.Severity, integrationID)

	// Set override if non-global and enabled (defaults to true when nil)
	enableOverride := route.Spec.EnableOverride == nil || *route.Spec.EnableOverride
	if route.Spec.Type != "global" && enableOverride {
		log.Info("Setting integration override", "routeType", apiRouteType, "severity", route.Spec.Severity)
		if err := apiClient.SetIntegrationOverride(ctx, route.Spec.ClusterType, route.Spec.ClusterName,
			apiRouteType, route.Spec.Severity, true); err != nil {
			log.Error(err, "Failed to set integration override")
			meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
				Type:               condTypeReady,
				Status:             metav1.ConditionFalse,
				Reason:             "OverrideError",
				Message:            common.SafeConditionMsg("Failed to set override", err),
				ObservedGeneration: route.Generation,
			})
			if err := r.Status().Update(ctx, route); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	// Add route if it doesn't exist
	if !routeExists {
		log.Info("Adding integration route", "integrationID", integrationID, "routeType", apiRouteType, "severity", route.Spec.Severity)
		if err := apiClient.AddIntegrationRoute(ctx, route.Spec.ClusterType, route.Spec.ClusterName,
			apiRouteType, route.Spec.Severity, integrationID); err != nil {
			log.Error(err, "Failed to add integration route")
			meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
				Type:               condTypeReady,
				Status:             metav1.ConditionFalse,
				Reason:             "RouteError",
				Message:            common.SafeConditionMsg("Failed to add route", err),
				ObservedGeneration: route.Generation,
			})
			if err := r.Status().Update(ctx, route); err != nil {
				log.Error(err, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else {
		log.Info("Route already exists, skipping creation")
	}

	// Update status
	now := metav1.Now()
	if err := r.Get(ctx, req.NamespacedName, route); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	route.Status.IntegrationID = integrationID
	route.Status.LastSyncTime = &now
	route.Status.ObservedGeneration = route.Generation
	meta.SetStatusCondition(&route.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             "RouteSynced",
		Message:            "Alert route successfully synced with AxonOps",
		ObservedGeneration: route.Generation,
	})

	if err := r.Status().Update(ctx, route); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled AxonOpsAlertRoute")
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsAlertRouteReconciler) handleDeletion(ctx context.Context, route *alertsv1alpha1.AxonOpsAlertRoute) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(route, alertRouteFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting alert route from AxonOps",
		"integrationID", route.Status.IntegrationID,
		"routeType", route.Spec.Type,
		"severity", route.Spec.Severity)

	// Resolve AxonOps API client for deletion
	apiClient, err := ResolveAPIClient(ctx, r.Client, route.Namespace, route.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Get integrations to verify route exists before deletion
	integrations, err := apiClient.GetIntegrations(ctx, route.Spec.ClusterType, route.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get integrations for cleanup")
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		// Non-retryable error, proceed with finalizer removal
	} else {
		// Find integration ID
		integrationID, err := findIntegrationID(integrations, route.Spec.IntegrationName, route.Spec.IntegrationType)
		if err != nil {
			log.Info("Integration not found, route may already be deleted")
		} else {
			// Get API route type
			apiRouteType, err := getAPIRouteType(route.Spec.Type)
			if err != nil {
				log.Error(err, "Invalid route type during deletion")
			} else {
				// Remove the route
				log.Info("Calling AxonOps API to remove route", "integrationID", integrationID, "routeType", apiRouteType)
				if err := apiClient.RemoveIntegrationRoute(ctx, route.Spec.ClusterType, route.Spec.ClusterName,
					apiRouteType, route.Spec.Severity, integrationID); err != nil {
					log.Error(err, "Failed to remove route from AxonOps", "integrationID", integrationID)
					var apiErr *axonops.APIError
					if errors.As(err, &apiErr) && apiErr.IsRetryable() {
						return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
					}
				} else {
					log.Info("Successfully removed route from AxonOps API", "integrationID", integrationID)
				}
			}
		}
	}

	// Remove the finalizer
	controllerutil.RemoveFinalizer(route, alertRouteFinalizerName)
	if err := r.Update(ctx, route); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted alert route")
	return ctrl.Result{}, nil
}

// findIntegrationID looks up the integration ID by name and type
func findIntegrationID(integrations *axonops.IntegrationsResponse, intName, intType string) (string, error) {
	for _, def := range integrations.Definitions {
		if strings.EqualFold(def.Type, intType) && strings.EqualFold(def.Params["name"], intName) {
			return def.ID, nil
		}
	}
	return "", fmt.Errorf("integration %s of type %s not found", intName, intType)
}

// getAPIRouteType converts the CRD route type to the API URL-encoded type
func getAPIRouteType(routeType string) (string, error) {
	apiType, ok := routeTypeMap[routeType]
	if !ok {
		return "", fmt.Errorf("unknown route type: %s", routeType)
	}
	return apiType, nil
}

// checkRouteExists checks if a route already exists in the integrations response
func checkRouteExists(integrations *axonops.IntegrationsResponse, apiRouteType, severity, integrationID string) bool {
	// Decode the API route type for comparison (URL-decode %20 to space)
	decodedAPIRouteType := strings.ReplaceAll(apiRouteType, "%20", " ")
	for _, routing := range integrations.Routings {
		if routing.Type == decodedAPIRouteType {
			for _, route := range routing.Routing {
				if route.ID == integrationID && strings.EqualFold(route.Severity, severity) {
					return true
				}
			}
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsAlertRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsAlertRoute{}).
		Named("alerts-axonopsalertroute").
		Complete(r)
}
