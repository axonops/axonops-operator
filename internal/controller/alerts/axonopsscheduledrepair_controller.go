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
	scheduledRepairFinalizerName = "alerts.axonops.com/scheduled-repair-finalizer"
)

// AxonOpsScheduledRepairReconciler reconciles a AxonOpsScheduledRepair object
type AxonOpsScheduledRepairReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsscheduledrepairs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsscheduledrepairs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsscheduledrepairs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsScheduledRepair
func (r *AxonOpsScheduledRepairReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.scheduledrepair", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsscheduledrepair", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsscheduledrepair", resultStr).Inc()
	}()

	repair := &alertsv1alpha1.AxonOpsScheduledRepair{}
	if err := r.Get(ctx, req.NamespacedName, repair); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsScheduledRepair", "repair", req.NamespacedName)

	if repair.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, repair)
	}

	if !controllerutil.ContainsFinalizer(repair, scheduledRepairFinalizerName) {
		controllerutil.AddFinalizer(repair, scheduledRepairFinalizerName)
		if err := r.Update(ctx, repair); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Validate mutual exclusivity
	if repair.Spec.SkipPaxos && repair.Spec.PaxosOnly {
		meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: repair.Generation,
			Reason:             "ValidationError",
			Message:            "SkipPaxos and PaxosOnly are mutually exclusive",
		})
		repair.Status.ObservedGeneration = repair.Generation
		if err := r.Status().Update(ctx, repair); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := ResolveAPIClient(ctx, r.Client, repair.Namespace, repair.Spec.ConnectionRef)
	if errors.Is(err, ErrConnectionPaused) {
		return HandleConnectionPaused(ctx, r.Client, repair, &repair.Status.Conditions)
	}
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: repair.Generation,
			Reason:             "FailedToResolveConnection",
			Message:            fmt.Sprintf("Failed to resolve connection: %v", err),
		})
		repair.Status.ObservedGeneration = repair.Generation
		if err := r.Status().Update(ctx, repair); err != nil {
			log.Error(err, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(repair.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		repair.Status.ObservedGeneration == repair.Generation &&
		repair.Status.SyncedRepairID != "" {
		log.Info("Repair already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	// If updating, delete the old repair first
	if repair.Status.SyncedRepairID != "" {
		log.Info("Deleting existing repair before recreate", "repairID", repair.Status.SyncedRepairID)
		if err := apiClient.DeleteScheduledRepair(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName, repair.Status.SyncedRepairID); err != nil {
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	}

	// Build and create the repair
	params := buildRepairParams(repair)

	if err := apiClient.CreateScheduledRepair(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName, params); err != nil {
		log.Error(err, "Failed to create scheduled repair")
		meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
			Type:               "Failed",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: repair.Generation,
			Reason:             "SyncFailed",
			Message:            fmt.Sprintf("Failed to sync with AxonOps: %v", err),
		})
		repair.Status.ObservedGeneration = repair.Generation
		if err := r.Status().Update(ctx, repair); err != nil {
			log.Error(err, "Failed to update status")
		}
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// GET all repairs to find the new one by tag and get its ID
	repairs, err := apiClient.GetScheduledRepairs(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName)
	if err != nil {
		log.Error(err, "Failed to get scheduled repairs after create")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	syncedID := findRepairIDByTag(repairs, repair.Spec.Tag)

	// Re-fetch CR and update status
	syncedGeneration := repair.Generation
	if err := r.Get(ctx, req.NamespacedName, repair); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	repair.Status.SyncedRepairID = syncedID
	now := metav1.Now()
	repair.Status.LastSyncTime = &now
	repair.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&repair.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Scheduled repair synced with AxonOps",
	})
	meta.RemoveStatusCondition(&repair.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, repair); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced scheduled repair", "repairID", syncedID, "tag", repair.Spec.Tag)
	return ctrl.Result{}, nil
}

func (r *AxonOpsScheduledRepairReconciler) handleDeletion(ctx context.Context, repair *alertsv1alpha1.AxonOpsScheduledRepair) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(repair, scheduledRepairFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting scheduled repair from AxonOps", "repairID", repair.Status.SyncedRepairID, "tag", repair.Spec.Tag)

	apiClient, err := ResolveAPIClient(ctx, r.Client, repair.Namespace, repair.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if repair.Status.SyncedRepairID != "" {
		if err := apiClient.DeleteScheduledRepair(ctx, repair.Spec.ClusterType, repair.Spec.ClusterName, repair.Status.SyncedRepairID); err != nil {
			log.Error(err, "Failed to delete repair from AxonOps", "repairID", repair.Status.SyncedRepairID)
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted repair from AxonOps API", "repairID", repair.Status.SyncedRepairID)
		}
	} else {
		log.Info("SyncedRepairID is empty, skipping API deletion")
	}

	controllerutil.RemoveFinalizer(repair, scheduledRepairFinalizerName)
	if err := r.Update(ctx, repair); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// buildRepairParams constructs the API payload from the CR spec
func buildRepairParams(repair *alertsv1alpha1.AxonOpsScheduledRepair) axonops.ScheduledRepairParams {
	params := axonops.ScheduledRepairParams{
		Tag:                 repair.Spec.Tag,
		Keyspace:            repair.Spec.Keyspace,
		Tables:              repair.Spec.Tables,
		BlacklistedTables:   repair.Spec.BlacklistedTables,
		Nodes:               repair.Spec.Nodes,
		SpecificDataCenters: repair.Spec.SpecificDataCenters,
		SegmentsPerNode:     repair.Spec.SegmentsPerNode,
		Segmented:           repair.Spec.Segmented,
		Incremental:         repair.Spec.Incremental,
		JobThreads:          repair.Spec.JobThreads,
		Schedule:            true,
		ScheduleExpr:        repair.Spec.ScheduleExpression,
		PrimaryRange:        repair.Spec.PrimaryRange,
		Parallelism:         repair.Spec.Parallelism,
		OptimiseStreams:     repair.Spec.OptimiseStreams,
		SkipPaxos:           repair.Spec.SkipPaxos,
		PaxosOnly:           repair.Spec.PaxosOnly,
	}

	// Ensure nil slices are empty arrays for JSON
	if params.Tables == nil {
		params.Tables = []string{}
	}
	if params.BlacklistedTables == nil {
		params.BlacklistedTables = []string{}
	}
	if params.Nodes == nil {
		params.Nodes = []string{}
	}
	if params.SpecificDataCenters == nil {
		params.SpecificDataCenters = []string{}
	}

	// Set default parallelism
	if params.Parallelism == "" {
		params.Parallelism = "Parallel"
	}
	if params.SegmentsPerNode == 0 {
		params.SegmentsPerNode = 1
	}
	if params.JobThreads == 0 {
		params.JobThreads = 1
	}

	// Transform paxos fields
	switch {
	case repair.Spec.SkipPaxos:
		params.Paxos = "Skip Paxos"
	case repair.Spec.PaxosOnly:
		params.Paxos = "Paxos Only"
	default:
		params.Paxos = "Default"
	}

	return params
}

// findRepairIDByTag searches the scheduled repairs response for a repair with the given tag
func findRepairIDByTag(repairs *axonops.ScheduledRepairsResponse, tag string) string {
	if repairs == nil {
		return ""
	}
	for _, entry := range repairs.Repairs {
		if len(entry.Params) > 0 && entry.Params[0].Tag == tag {
			return entry.ID
		}
	}
	return ""
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsScheduledRepairReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsScheduledRepair{}).
		Named("alerts-axonopsscheduledrepair").
		Complete(r)
}
