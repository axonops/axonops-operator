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

package kafka

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

	kafkav1alpha1 "github.com/axonops/axonops-operator/api/kafka/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
	"github.com/axonops/axonops-operator/internal/controller/common"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
)

const (
	kafkaACLFinalizerName = "kafka.axonops.com/acl-finalizer"
)

// AxonOpsKafkaACLReconciler reconciles a AxonOpsKafkaACL object
type AxonOpsKafkaACLReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaacls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaacls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaacls/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsKafkaACL
func (r *AxonOpsKafkaACLReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.kafkaacl", trace.WithAttributes())
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
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopskafkaacl", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopskafkaacl", resultStr).Inc()
	}()

	acl := &kafkav1alpha1.AxonOpsKafkaACL{}
	if err := r.Get(ctx, req.NamespacedName, acl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsKafkaACL", "acl", req.NamespacedName)

	if acl.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, acl)
	}

	if !controllerutil.ContainsFinalizer(acl, kafkaACLFinalizerName) {
		controllerutil.AddFinalizer(acl, kafkaACLFinalizerName)
		if err := r.Update(ctx, acl); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, acl.Namespace, acl.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, acl, "FailedToResolveConnection", fmt.Sprintf("Failed to resolve connection: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(acl.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		acl.Status.ObservedGeneration == acl.Generation &&
		acl.Status.Synced {
		log.Info("ACL already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	aclPayload := buildACLPayload(acl)

	if err := apiClient.CreateKafkaACL(ctx, acl.Spec.ClusterName, aclPayload); err != nil {
		log.Error(err, "Failed to create Kafka ACL")
		r.setFailedCondition(ctx, acl, "CreateFailed", fmt.Sprintf("Failed to create ACL: %v", err))
		var apiErr *axonops.APIError
		if errors.As(err, &apiErr) && apiErr.IsRetryable() {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Re-fetch and update status
	syncedGeneration := acl.Generation
	if err := r.Get(ctx, req.NamespacedName, acl); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	acl.Status.Synced = true
	now := metav1.Now()
	acl.Status.LastSyncTime = &now
	acl.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&acl.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Kafka ACL synced with AxonOps",
	})
	meta.RemoveStatusCondition(&acl.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, acl); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced Kafka ACL", "principal", acl.Spec.Principal, "operation", acl.Spec.Operation)
	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaACLReconciler) handleDeletion(ctx context.Context, acl *kafkav1alpha1.AxonOpsKafkaACL) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(acl, kafkaACLFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting Kafka ACL from AxonOps", "principal", acl.Spec.Principal)

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, acl.Namespace, acl.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if acl.Status.Synced {
		aclPayload := buildACLPayload(acl)
		if err := apiClient.DeleteKafkaACL(ctx, acl.Spec.ClusterName, aclPayload); err != nil {
			log.Error(err, "Failed to delete Kafka ACL")
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted Kafka ACL from AxonOps")
		}
	}

	controllerutil.RemoveFinalizer(acl, kafkaACLFinalizerName)
	if err := r.Update(ctx, acl); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaACLReconciler) setFailedCondition(ctx context.Context, acl *kafkav1alpha1.AxonOpsKafkaACL, reason, message string) {
	meta.SetStatusCondition(&acl.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: acl.Generation,
		Reason:             reason,
		Message:            message,
	})
	acl.Status.ObservedGeneration = acl.Generation
	if err := r.Status().Update(ctx, acl); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

func buildACLPayload(acl *kafkav1alpha1.AxonOpsKafkaACL) axonops.KafkaACL {
	patternType := acl.Spec.ResourcePatternType
	if patternType == "" {
		patternType = "LITERAL"
	}
	host := acl.Spec.Host
	if host == "" {
		host = "*"
	}

	return axonops.KafkaACL{
		ResourceType:        acl.Spec.ResourceType,
		ResourceName:        acl.Spec.ResourceName,
		ResourcePatternType: patternType,
		Principal:           acl.Spec.Principal,
		Host:                host,
		Operation:           acl.Spec.Operation,
		PermissionType:      acl.Spec.PermissionType,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsKafkaACLReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kafkav1alpha1.AxonOpsKafkaACL{}).
		Named("kafka-axonopskafkaacl").
		Complete(r)
}
