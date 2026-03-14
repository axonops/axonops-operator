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
	shellHealthcheckFinalizer = "alerts.axonops.com/shell-healthcheck-finalizer"
)

// AxonOpsHealthcheckShellReconciler reconciles a AxonOpsHealthcheckShell object
type AxonOpsHealthcheckShellReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckshells,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckshells/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthcheckshells/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsHealthcheckShell
func (r *AxonOpsHealthcheckShellReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsHealthcheckShell CR
	healthcheck := &alertsv1alpha1.AxonOpsHealthcheckShell{}
	if err := r.Get(ctx, req.NamespacedName, healthcheck); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsHealthcheckShell", "healthcheck", req.NamespacedName)

	// Handle deletion
	if healthcheck.ObjectMeta.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, healthcheck)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(healthcheck, shellHealthcheckFinalizer) {
		controllerutil.AddFinalizer(healthcheck, shellHealthcheckFinalizer)
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
	for i, existing := range allHealthchecks.Shell {
		if existing.ID == healthcheck.Status.SyncedHealthcheckID {
			allHealthchecks.Shell[i] = entry
			found = true
			break
		}
	}
	if !found {
		// Generate new ID for new healthcheck
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}
		allHealthchecks.Shell = append(allHealthchecks.Shell, entry)
	}

	// Update all healthchecks via bulk PUT
	if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
		log.Error(err, "Failed to update healthchecks")
		r.setFailedCondition(ctx, healthcheck, ReasonAPIError, fmt.Sprintf("Failed to update healthchecks: %v", err))
		if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
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

	log.Info("Successfully synced Shell healthcheck", "healthcheckID", syncedID)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsHealthcheckShellReconciler) handleDeletion(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckShell) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(healthcheck, shellHealthcheckFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting Shell healthcheck from AxonOps", "healthcheckID", healthcheck.Status.SyncedHealthcheckID)

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
			if apiErr, ok := err.(*axonops.APIError); ok && apiErr.StatusCode == 404 {
				log.Info("Healthchecks not found, proceeding with finalizer removal")
			} else {
				log.Error(err, "Failed to get healthchecks for deletion")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			// Remove our healthcheck from the list
			newList := make([]axonops.HealthcheckShell, 0, len(allHealthchecks.Shell))
			for _, h := range allHealthchecks.Shell {
				if h.ID != healthcheck.Status.SyncedHealthcheckID {
					newList = append(newList, h)
				}
			}
			allHealthchecks.Shell = newList

			// Update via bulk PUT
			if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
				if apiErr, ok := err.(*axonops.APIError); ok && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				log.Error(err, "Failed to delete healthcheck from AxonOps")
			} else {
				log.Info("Successfully deleted Shell healthcheck from AxonOps")
			}
		}
	} else {
		log.Info("SyncedHealthcheckID is empty, skipping API deletion")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(healthcheck, shellHealthcheckFinalizer)
	if err := r.Update(ctx, healthcheck); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildHealthcheckEntry builds an API healthcheck entry from the CR spec
func (r *AxonOpsHealthcheckShellReconciler) buildHealthcheckEntry(healthcheck *alertsv1alpha1.AxonOpsHealthcheckShell) axonops.HealthcheckShell {
	entry := axonops.HealthcheckShell{
		Name:     healthcheck.Spec.Name,
		Script:   healthcheck.Spec.Script,
		Shell:    healthcheck.Spec.Shell,
		Interval: healthcheck.Spec.Interval,
		Timeout:  healthcheck.Spec.Timeout,
		Readonly: healthcheck.Spec.Readonly,
	}

	// Use existing ID if we have one
	if healthcheck.Status.SyncedHealthcheckID != "" {
		entry.ID = healthcheck.Status.SyncedHealthcheckID
	}

	return entry
}

// setFailedCondition sets a failed condition on the healthcheck
func (r *AxonOpsHealthcheckShellReconciler) setFailedCondition(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckShell, reason, message string) {
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
func (r *AxonOpsHealthcheckShellReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsHealthcheckShell{}).
		Named("alerts-axonopshealthcheckshell").
		Complete(r)
}
