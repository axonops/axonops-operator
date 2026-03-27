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
	logCollectorFinalizerName = "alerts.axonops.com/log-collector-finalizer"
)

// AxonOpsLogCollectorReconciler reconciles a AxonOpsLogCollector object
type AxonOpsLogCollectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogcollectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogcollectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopslogcollectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsLogCollector
func (r *AxonOpsLogCollectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.logcollector", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopslogcollector", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopslogcollector", resultStr).Inc()
	}()

	cr := &alertsv1alpha1.AxonOpsLogCollector{}
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsLogCollector", "logcollector", req.NamespacedName)

	if cr.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, cr)
	}

	if !controllerutil.ContainsFinalizer(cr, logCollectorFinalizerName) {
		controllerutil.AddFinalizer(cr, logCollectorFinalizerName)
		if err := r.Update(ctx, cr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := ResolveAPIClient(ctx, r.Client, cr.Namespace, cr.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             "FailedToResolveConnection",
			Message:            common.SafeConditionMsg("Failed to resolve connection", err),
		})
		cr.Status.ObservedGeneration = cr.Generation
		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(cr.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		cr.Status.ObservedGeneration == cr.Generation &&
		cr.Status.SyncedUUID != "" {
		log.Info("Log collector already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// GET current list of log collectors
	existing, err := apiClient.GetLogCollectors(ctx, cr.Spec.ClusterType, cr.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get log collectors")
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             "SyncFailed",
			Message:            common.SafeConditionMsg("Failed to get log collectors", err),
		})
		cr.Status.ObservedGeneration = cr.Generation
		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update status")
		}
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Separate matching collector from others, reuse UUID if found
	var others []axonops.LogCollector
	collectorUUID := uuid.New().String()
	for _, lc := range existing {
		if lc.Filename == cr.Spec.Filename {
			collectorUUID = lc.UUID
		} else {
			others = append(others, lc)
		}
	}

	// Build the updated collector entry
	updated := axonops.LogCollector{
		UUID:         collectorUUID,
		ID:           "",
		Name:         cr.Spec.Name,
		Interval:     cr.Spec.Interval,
		Timeout:      cr.Spec.Timeout,
		Filename:     cr.Spec.Filename,
		DateFormat:   cr.Spec.DateFormat,
		InfoRegex:    cr.Spec.InfoRegex,
		WarningRegex: cr.Spec.WarningRegex,
		ErrorRegex:   cr.Spec.ErrorRegex,
		DebugRegex:   cr.Spec.DebugRegex,
		Readonly:     cr.Spec.Readonly,
		Integrations: axonops.LogCollectorIntegrations{
			OverrideError:   false,
			OverrideInfo:    false,
			OverrideWarning: false,
			Routing:         nil,
			Type:            "",
		},
	}

	collectors := append(others, updated)

	// PUT the full list
	if err := apiClient.PutLogCollectors(ctx, cr.Spec.ClusterType, cr.Spec.ClusterName, collectors); err != nil {
		log.Error(err, "Failed to put log collectors")
		meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: cr.Generation,
			Reason:             "SyncFailed",
			Message:            common.SafeConditionMsg("Failed to sync log collectors", err),
		})
		cr.Status.ObservedGeneration = cr.Generation
		if err := r.Status().Update(ctx, cr); err != nil {
			log.Error(err, "Failed to update status")
		}
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Re-fetch and update status
	syncedGeneration := cr.Generation
	if err := r.Get(ctx, req.NamespacedName, cr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cr.Status.SyncedUUID = collectorUUID
	now := metav1.Now()
	cr.Status.LastSyncTime = &now
	cr.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&cr.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Log collector synced with AxonOps",
	})
	meta.RemoveStatusCondition(&cr.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced log collector", "uuid", collectorUUID)
	return ctrl.Result{}, nil
}

func (r *AxonOpsLogCollectorReconciler) handleDeletion(ctx context.Context, cr *alertsv1alpha1.AxonOpsLogCollector) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(cr, logCollectorFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting log collector from AxonOps", "filename", cr.Spec.Filename)

	apiClient, err := ResolveAPIClient(ctx, r.Client, cr.Namespace, cr.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// GET the current list
	existing, err := apiClient.GetLogCollectors(ctx, cr.Spec.ClusterType, cr.Spec.ClusterName)
	if err != nil {
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			log.Error(err, "Failed to get log collectors for deletion — will retry")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		// Non-retryable error — proceed with finalizer removal
		log.Error(err, "Failed to get log collectors for deletion — proceeding with cleanup")
	} else {
		// Filter out the collector matching our filename
		var filtered []axonops.LogCollector
		for _, lc := range existing {
			if lc.Filename != cr.Spec.Filename {
				filtered = append(filtered, lc)
			}
		}

		// Only PUT if we actually removed something
		if len(filtered) != len(existing) {
			if err := apiClient.PutLogCollectors(ctx, cr.Spec.ClusterType, cr.Spec.ClusterName, filtered); err != nil {
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					log.Error(err, "Failed to put log collectors for deletion — will retry")
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				log.Error(err, "Failed to put log collectors for deletion — proceeding with cleanup")
			} else {
				log.Info("Successfully removed log collector from AxonOps")
			}
		}
	}

	controllerutil.RemoveFinalizer(cr, logCollectorFinalizerName)
	if err := r.Update(ctx, cr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsLogCollectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsLogCollector{}).
		Named("alerts-axonopslogcollector").
		Complete(r)
}
