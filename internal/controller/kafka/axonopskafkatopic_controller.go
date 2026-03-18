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
)

const (
	kafkaTopicFinalizerName = "kafka.axonops.com/topic-finalizer"
	condTypeReady           = "Ready"
)

// AxonOpsKafkaTopicReconciler reconciles a AxonOpsKafkaTopic object
type AxonOpsKafkaTopicReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkatopics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkatopics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kafka.axonops.com,resources=axonopskafkatopics/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile implements the reconciliation loop for AxonOpsKafkaTopic
func (r *AxonOpsKafkaTopicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	topic := &kafkav1alpha1.AxonOpsKafkaTopic{}
	if err := r.Get(ctx, req.NamespacedName, topic); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsKafkaTopic", "topic", req.NamespacedName)

	if topic.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, topic)
	}

	if !controllerutil.ContainsFinalizer(topic, kafkaTopicFinalizerName) {
		controllerutil.AddFinalizer(topic, kafkaTopicFinalizerName)
		if err := r.Update(ctx, topic); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, topic.Namespace, topic.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve AxonOps API client")
		r.setFailedCondition(ctx, topic, "FailedToResolveConnection", fmt.Sprintf("Failed to resolve connection: %v", err))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Check idempotency
	readyCond := meta.FindStatusCondition(topic.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		topic.Status.ObservedGeneration == topic.Generation &&
		topic.Status.Synced {
		log.Info("Topic already synced and spec unchanged, skipping API call")
		return ctrl.Result{}, nil
	}

	if !topic.Status.Synced {
		// Create the topic
		createReq := axonops.KafkaTopicCreateRequest{
			TopicName:         topic.Spec.Name,
			PartitionCount:    topic.Spec.Partitions,
			ReplicationFactor: topic.Spec.ReplicationFactor,
			Configs:           mapToTopicConfigs(topic.Spec.Config),
		}

		if err := apiClient.CreateKafkaTopic(ctx, topic.Spec.ClusterName, createReq); err != nil {
			log.Error(err, "Failed to create Kafka topic")
			r.setFailedCondition(ctx, topic, "CreateFailed", fmt.Sprintf("Failed to create topic: %v", err))
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			return ctrl.Result{}, nil
		}
	} else {
		// Topic exists — update config only (partitions/RF are immutable)
		configs := make([]axonops.KafkaTopicConfigUpdate, 0, len(topic.Spec.Config))
		for k, v := range topic.Spec.Config {
			configs = append(configs, axonops.KafkaTopicConfigUpdate{
				Key: k, Value: v, Op: "SET",
			})
		}
		if len(configs) > 0 {
			if err := apiClient.UpdateKafkaTopicConfig(ctx, topic.Spec.ClusterName, topic.Spec.Name, configs); err != nil {
				log.Error(err, "Failed to update Kafka topic config")
				r.setFailedCondition(ctx, topic, "UpdateFailed", fmt.Sprintf("Failed to update topic config: %v", err))
				var apiErr *axonops.APIError
				if errors.As(err, &apiErr) && apiErr.IsRetryable() {
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				return ctrl.Result{}, nil
			}
		}
	}

	// Re-fetch and update status
	syncedGeneration := topic.Generation
	if err := r.Get(ctx, req.NamespacedName, topic); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	topic.Status.Synced = true
	now := metav1.Now()
	topic.Status.LastSyncTime = &now
	topic.Status.ObservedGeneration = syncedGeneration

	meta.SetStatusCondition(&topic.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: syncedGeneration,
		Reason:             "Synced",
		Message:            "Kafka topic synced with AxonOps",
	})
	meta.RemoveStatusCondition(&topic.Status.Conditions, "Failed")

	if err := r.Status().Update(ctx, topic); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Successfully synced Kafka topic", "topic", topic.Spec.Name)
	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaTopicReconciler) handleDeletion(ctx context.Context, topic *kafkav1alpha1.AxonOpsKafkaTopic) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(topic, kafkaTopicFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting Kafka topic from AxonOps", "topic", topic.Spec.Name)

	apiClient, err := common.ResolveAPIClient(ctx, r.Client, topic.Namespace, topic.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion — will retry")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if topic.Status.Synced {
		if err := apiClient.DeleteKafkaTopic(ctx, topic.Spec.ClusterName, topic.Spec.Name); err != nil {
			log.Error(err, "Failed to delete Kafka topic")
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		} else {
			log.Info("Successfully deleted Kafka topic from AxonOps")
		}
	}

	controllerutil.RemoveFinalizer(topic, kafkaTopicFinalizerName)
	if err := r.Update(ctx, topic); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AxonOpsKafkaTopicReconciler) setFailedCondition(ctx context.Context, topic *kafkav1alpha1.AxonOpsKafkaTopic, reason, message string) {
	meta.SetStatusCondition(&topic.Status.Conditions, metav1.Condition{
		Type:               "Failed",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: topic.Generation,
		Reason:             reason,
		Message:            message,
	})
	topic.Status.ObservedGeneration = topic.Generation
	if err := r.Status().Update(ctx, topic); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to update status")
	}
}

// mapToTopicConfigs converts a map to the API config format
func mapToTopicConfigs(config map[string]string) []axonops.KafkaTopicConfig {
	configs := make([]axonops.KafkaTopicConfig, 0, len(config))
	for k, v := range config {
		configs = append(configs, axonops.KafkaTopicConfig{Name: k, Value: v})
	}
	return configs
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsKafkaTopicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kafkav1alpha1.AxonOpsKafkaTopic{}).
		Named("kafka-axonopskafkatopic").
		Complete(r)
}
