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
	httpHealthcheckFinalizer = "alerts.axonops.com/http-healthcheck-finalizer"
)

// AxonOpsHealthcheckHTTPReconciler reconciles a AxonOpsHealthcheckHTTP object
type AxonOpsHealthcheckHTTPReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckhttps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckhttps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckhttps/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsHealthcheckHTTP
func (r *AxonOpsHealthcheckHTTPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsHealthcheckHTTP CR
	healthcheck := &alertsv1alpha1.AxonOpsHealthcheckHTTP{}
	if err := r.Get(ctx, req.NamespacedName, healthcheck); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsHealthcheckHTTP", "healthcheck", req.NamespacedName)

	// Handle deletion
	if healthcheck.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, healthcheck)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(healthcheck, httpHealthcheckFinalizer) {
		controllerutil.AddFinalizer(healthcheck, httpHealthcheckFinalizer)
		if err := r.Update(ctx, healthcheck); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, healthcheck.Namespace, healthcheck.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, healthcheck, ReasonConnectionError, fmt.Sprintf("Failed to resolve connection: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check if healthcheck is already synced
	readyCond := meta.FindStatusCondition(healthcheck.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		healthcheck.Status.ObservedGeneration == healthcheck.Generation &&
		healthcheck.Status.SyncedHealthcheckID != "" {
		log.Info("Healthcheck already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// Get all healthchecks from API
	allHealthchecks, err := apiClient.GetHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get healthchecks from AxonOps")
		r.setFailedCondition(ctx, healthcheck, ReasonAPIError, fmt.Sprintf("Failed to get healthchecks: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build the healthcheck entry
	entry := r.buildHealthcheckEntry(healthcheck)

	// Find and update or add our healthcheck
	found := false
	for i, existing := range allHealthchecks.HTTPChecks {
		if existing.ID == healthcheck.Status.SyncedHealthcheckID {
			allHealthchecks.HTTPChecks[i] = entry
			found = true
			break
		}
	}
	if !found {
		// Generate new ID for new healthcheck
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}
		allHealthchecks.HTTPChecks = append(allHealthchecks.HTTPChecks, entry)
	}

	// Update all healthchecks via bulk PUT
	if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
		log.Error(err, "Failed to update healthchecks")
		r.setFailedCondition(ctx, healthcheck, ReasonAPIError, fmt.Sprintf("Failed to update healthchecks: %v", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Re-fetch CR and update status
	syncedID := entry.ID
	syncedGeneration := healthcheck.Generation
	if err := r.Get(ctx, req.NamespacedName, healthcheck); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	healthcheck.Status.SyncedHealthcheckID = syncedID
	now := metav1.Now()
	healthcheck.Status.LastSyncTime = &now
	healthcheck.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&healthcheck.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Healthcheck synced with AxonOps",
	})
	meta.RemoveStatusCondition(&healthcheck.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, healthcheck); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced HTTP healthcheck", "healthcheckID", syncedID)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsHealthcheckHTTPReconciler) handleDeletion(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckHTTP) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(healthcheck, httpHealthcheckFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting HTTP healthcheck from AxonOps", "healthcheckID", healthcheck.Status.SyncedHealthcheckID)

	// Resolve API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, healthcheck.Namespace, healthcheck.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Only attempt API deletion if we have a synced ID
	if healthcheck.Status.SyncedHealthcheckID != "" {
		// Get all healthchecks
		allHealthchecks, err := apiClient.GetHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName)
		if err != nil {
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
				log.Info("Healthchecks not found, proceeding with finalizer removal")
			} else {
				log.Error(err, "Failed to get healthchecks for deletion")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			// Remove our healthcheck from the list
			newList := make([]axonops.HealthcheckHTTP, 0, len(allHealthchecks.HTTPChecks))
			for _, h := range allHealthchecks.HTTPChecks {
				if h.ID != healthcheck.Status.SyncedHealthcheckID {
					newList = append(newList, h)
				}
			}
			allHealthchecks.HTTPChecks = newList

			// Update via bulk PUT
			if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				log.Error(err, "Failed to delete healthcheck from AxonOps")
			} else {
				log.Info("Successfully deleted HTTP healthcheck from AxonOps")
			}
		}
	} else {
		log.Info("SyncedHealthcheckID is empty, skipping API deletion")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(healthcheck, httpHealthcheckFinalizer)
	if err := r.Update(ctx, healthcheck); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildHealthcheckEntry builds an API healthcheck entry from the CR spec
func (r *AxonOpsHealthcheckHTTPReconciler) buildHealthcheckEntry(healthcheck *alertsv1alpha1.AxonOpsHealthcheckHTTP) axonops.HealthcheckHTTP {
	// Convert headers map to []string format ("Key: Value")
	headers := make([]string, 0, len(healthcheck.Spec.Headers))
	for k, v := range healthcheck.Spec.Headers {
		headers = append(headers, k+": "+v)
	}

	entry := axonops.HealthcheckHTTP{
		Name:                healthcheck.Spec.Name,
		HTTP:                healthcheck.Spec.URL,
		Method:              healthcheck.Spec.Method,
		Headers:             headers,
		Body:                healthcheck.Spec.Body,
		ExpectedStatus:      healthcheck.Spec.ExpectedStatus,
		Interval:            healthcheck.Spec.Interval,
		Timeout:             healthcheck.Spec.Timeout,
		Readonly:            healthcheck.Spec.Readonly,
		TLSSkipVerify:       healthcheck.Spec.TLSSkipVerify,
		SupportedAgentTypes: healthcheck.Spec.SupportedAgentTypes,
	}

	// Use existing ID if we have one
	if healthcheck.Status.SyncedHealthcheckID != "" {
		entry.ID = healthcheck.Status.SyncedHealthcheckID
	}

	return entry
}

// setFailedCondition sets a failed condition on the healthcheck
func (r *AxonOpsHealthcheckHTTPReconciler) setFailedCondition(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckHTTP, reason, message string) {
	meta.SetStatusCondition(&healthcheck.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: healthcheck.Generation,
		Reason:             reason,
		Message:            message,
	})
	healthcheck.Status.ObservedGeneration = healthcheck.Generation
	if err := r.Status().Update(ctx, healthcheck); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsHealthcheckHTTPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsHealthcheckHTTP{}).
		Named("alerts-axonopshealthcheckhttp").
		Complete(r)
}
