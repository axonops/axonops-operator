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
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	tcpHealthcheckFinalizer = "alerts.axonops.com/tcp-healthcheck-finalizer"
)

// AxonOpsHealthcheckTCPReconciler reconciles a AxonOpsHealthcheckTCP object
type AxonOpsHealthcheckTCPReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthchecktcps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthchecktcps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopshealthchecktcps/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsHealthcheckTCP
func (r *AxonOpsHealthcheckTCPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.healthchecktcp", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopshealthchecktcp", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopshealthchecktcp", resultStr).Inc()
	}()

	// Fetch the AxonOpsHealthcheckTCP CR
	healthcheck := &alertsv1alpha1.AxonOpsHealthcheckTCP{}
	if err := r.Get(ctx, req.NamespacedName, healthcheck); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsHealthcheckTCP", "healthcheck", req.NamespacedName)

	// Handle deletion
	if healthcheck.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, healthcheck)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(healthcheck, tcpHealthcheckFinalizer) {
		controllerutil.AddFinalizer(healthcheck, tcpHealthcheckFinalizer)
		if err := r.Update(ctx, healthcheck); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Resolve AxonOps API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, healthcheck.Namespace, healthcheck.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, healthcheck, &healthcheck.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, healthcheck, ReasonConnectionError, common.SafeConditionMsg("Failed to resolve connection", err))
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
		r.setFailedCondition(ctx, healthcheck, ReasonAPIError, common.SafeConditionMsg("Failed to get healthchecks", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build the healthcheck entry
	entry := r.buildHealthcheckEntry(healthcheck)

	// Find and update or add our healthcheck
	found := false
	for i, existing := range allHealthchecks.TCPChecks {
		if existing.ID == healthcheck.Status.SyncedHealthcheckID {
			allHealthchecks.TCPChecks[i] = entry
			found = true
			break
		}
	}
	if !found {
		// Generate new ID for new healthcheck
		if entry.ID == "" {
			entry.ID = uuid.New().String()
		}
		allHealthchecks.TCPChecks = append(allHealthchecks.TCPChecks, entry)
	}

	// Update all healthchecks via bulk PUT
	if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
		log.Error(err, "Failed to update healthchecks")
		r.setFailedCondition(ctx, healthcheck, ReasonAPIError, common.SafeConditionMsg("Failed to update healthchecks", err))
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

	log.Info("Successfully synced TCP healthcheck", "healthcheckID", syncedID)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsHealthcheckTCPReconciler) handleDeletion(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckTCP) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(healthcheck, tcpHealthcheckFinalizer) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting TCP healthcheck from AxonOps", "healthcheckID", healthcheck.Status.SyncedHealthcheckID)

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
			newList := make([]axonops.HealthcheckTCP, 0, len(allHealthchecks.TCPChecks))
			for _, h := range allHealthchecks.TCPChecks {
				if h.ID != healthcheck.Status.SyncedHealthcheckID {
					newList = append(newList, h)
				}
			}
			allHealthchecks.TCPChecks = newList

			// Update via bulk PUT
			if err := apiClient.UpdateHealthchecks(ctx, healthcheck.Spec.ClusterType, healthcheck.Spec.ClusterName, allHealthchecks); err != nil {
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				log.Error(err, "Failed to delete healthcheck from AxonOps")
			} else {
				log.Info("Successfully deleted TCP healthcheck from AxonOps")
			}
		}
	} else {
		log.Info("SyncedHealthcheckID is empty, skipping API deletion")
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(healthcheck, tcpHealthcheckFinalizer)
	if err := r.Update(ctx, healthcheck); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildHealthcheckEntry builds an API healthcheck entry from the CR spec
func (r *AxonOpsHealthcheckTCPReconciler) buildHealthcheckEntry(healthcheck *alertsv1alpha1.AxonOpsHealthcheckTCP) axonops.HealthcheckTCP {
	entry := axonops.HealthcheckTCP{
		Name:                healthcheck.Spec.Name,
		TCP:                 healthcheck.Spec.TCP,
		Interval:            healthcheck.Spec.Interval,
		Timeout:             healthcheck.Spec.Timeout,
		Readonly:            healthcheck.Spec.Readonly,
		SupportedAgentTypes: healthcheck.Spec.SupportedAgentTypes,
	}

	// Use existing ID if we have one
	if healthcheck.Status.SyncedHealthcheckID != "" {
		entry.ID = healthcheck.Status.SyncedHealthcheckID
	}

	return entry
}

// setFailedCondition sets a failed condition on the healthcheck
func (r *AxonOpsHealthcheckTCPReconciler) setFailedCondition(ctx context.Context, healthcheck *alertsv1alpha1.AxonOpsHealthcheckTCP, reason, message string) {
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
func (r *AxonOpsHealthcheckTCPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsHealthcheckTCP{}).
		Named("alerts-axonopshealthchecktcp").
		Complete(r)
}
