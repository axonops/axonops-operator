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
	kafkaConnectorFinalizerName = "kafka.axonops.com/connector-finalizer"
)

// AxonOpsKafkaConnectorReconciler reconciles a AxonOpsKafkaConnector object
type AxonOpsKafkaConnectorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaconnectors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaconnectors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkaconnectors/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AxonOpsKafkaConnectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.kafkaconnector", trace.WithAttributes())
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	start := time.Now()
	defer func() {
		resultStr := "success"
		if err != nil {
			resultStr = "error"
			axonopsmetrics.ReconcileErrorsTotal.WithLabelValues(axonopsmetrics.ClassifyError(err)).Inc()
		}
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopskafkaconnector", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopskafkaconnector", resultStr).Inc()
	}()

	connector := &kafkav1alpha1.AxonOpsKafkaConnector{}
	if err := r.Get(ctx, req.NamespacedName, connector); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsKafkaConnector", "connector", req.NamespacedName)

	if connector.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, connector)
	}

	if !controllerutil.ContainsFinalizer(connector, kafkaConnectorFinalizerName) {
		controllerutil.AddFinalizer(connector, kafkaConnectorFinalizerName)
		if err := r.Update(ctx, connector); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, connector.Namespace, connector.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, connector, "FailedToResolveConnection", fmt.Sprintf("Failed to resolve connection: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(connector.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		connector.Status.ObservedGeneration == connector.Generation &&
		connector.Status.Synced {
		log.Info("Connector already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	if !connector.Status.Synced {
		// Create the connector
		createReq := axonops.KafkaConnectorCreateRequest{
			Name:   connector.Spec.Name,
			Config: connector.Spec.Config,
		}
		resp, err := apiClient.CreateKafkaConnector(ctx, connector.Spec.ClusterName, connector.Spec.ConnectClusterName, createReq)
		if err != nil {
			log.Error(err, "Failed to create Kafka connector")
			r.setFailedCondition(ctx, connector, "CreateFailed", fmt.Sprintf("Failed to create connector: %v", err))
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, nil
		}
		// Store the connector type from the response
		if resp != nil {
			connector.Status.ConnectorType = resp.Type
		}
	} else {
		// Connector exists — update config only
		if err := apiClient.UpdateKafkaConnectorConfig(ctx, connector.Spec.ClusterName, connector.Spec.ConnectClusterName, connector.Spec.Name, connector.Spec.Config); err != nil {
			log.Error(err, "Failed to update Kafka connector config")
			r.setFailedCondition(ctx, connector, "UpdateFailed", fmt.Sprintf("Failed to update connector config: %v", err))
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, nil
		}
	}

	// Re-fetch and update status
	syncedGeneration := connector.Generation
	connectorType := connector.Status.ConnectorType
	if err := r.Get(ctx, req.NamespacedName, connector); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	connector.Status.Synced = true
	connector.Status.ConnectorType = connectorType
	now := metav1.Now()
	connector.Status.LastSyncTime = &now
	connector.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Kafka connector synced with AxonOps",
	})
	meta.RemoveStatusCondition(&connector.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, connector); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced Kafka connector", "connector", connector.Spec.Name)
	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaConnectorReconciler) handleDeletion(ctx context.Context, connector *kafkav1alpha1.AxonOpsKafkaConnector) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(connector, kafkaConnectorFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting Kafka connector from AxonOps", "connector", connector.Spec.Name)

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, connector.Namespace, connector.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if connector.Status.Synced {
		if err := apiClient.DeleteKafkaConnector(ctx, connector.Spec.ClusterName, connector.Spec.ConnectClusterName, connector.Spec.Name); err != nil {
			log.Error(err, "Failed to delete Kafka connector")
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted Kafka connector from AxonOps")
		}
	}

	controllerutil.RemoveFinalizer(connector, kafkaConnectorFinalizerName)
	if err := r.Update(ctx, connector); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaConnectorReconciler) setFailedCondition(ctx context.Context, connector *kafkav1alpha1.AxonOpsKafkaConnector, reason, message string) {
	meta.SetStatusCondition(&connector.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: connector.Generation,
		Reason:             reason,
		Message:            message,
	})
	connector.Status.ObservedGeneration = connector.Generation
	if err := r.Status().Update(ctx, connector); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsKafkaConnectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kafkav1alpha1.AxonOpsKafkaConnector{}).
		Named("kafka-axonopskafkaconnector").
		Complete(r)
}
