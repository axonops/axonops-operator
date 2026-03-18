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
	"maps"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

const (
	alertEndpointFinalizerName = "alerts.axonops.com/alert-endpoint-finalizer"
)

// Mapping from CRD type enum to the set of typed integration types
var typedIntegrationTypes = map[string]bool{
	"slack":           true,
	"pagerduty":       true,
	"opsgenie":        true,
	"servicenow":      true,
	"microsoft_teams": true,
}

// AxonOpsAlertEndpointReconciler reconciles a AxonOpsAlertEndpoint object
type AxonOpsAlertEndpointReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertendpoints,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertendpoints/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=alerts.axonops.com,resources=axonopsalertendpoints/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsconnections,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *AxonOpsAlertEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsAlertEndpoint instance
	endpoint := &alertsv1alpha1.AxonOpsAlertEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if endpoint.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, endpoint)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(endpoint, alertEndpointFinalizerName) {
		controllerutil.AddFinalizer(endpoint, alertEndpointFinalizerName)
		if err := r.Update(ctx, endpoint); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Idempotency check: skip if already synced and generation unchanged
	readyCond := meta.FindStatusCondition(endpoint.Status.Conditions, condTypeReady)
	if readyCond != nil && readyCond.Status == metav1.ConditionTrue &&
		endpoint.Status.ObservedGeneration == endpoint.Generation &&
		endpoint.Status.SyncedIntegrationID != "" {
		log.Info("Endpoint already synced, skipping reconciliation",
			"integrationID", endpoint.Status.SyncedIntegrationID, "generation", endpoint.Generation)
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling AxonOpsAlertEndpoint",
		"clusterName", endpoint.Spec.ClusterName,
		"type", endpoint.Spec.Type,
		"name", endpoint.Spec.Name)

	// Validate type-config consistency
	params, err := r.buildParams(ctx, endpoint)
	if err != nil {
		log.Error(err, "Failed to build integration params")
		meta.SetStatusCondition(&endpoint.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             err.(*endpointConfigError).Reason,
			Message:            err.Error(),
			ObservedGeneration: endpoint.Generation,
		})
		if statusErr := r.Status().Update(ctx, endpoint); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		if err.(*endpointConfigError).Retryable {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, nil
	}

	// Resolve API client
	apiClient, err := ResolveAPIClient(ctx, r.Client, endpoint.Namespace, endpoint.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client")
		meta.SetStatusCondition(&endpoint.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonConnectionError,
			Message:            fmt.Sprintf("Failed to resolve AxonOps connection: %v", err),
			ObservedGeneration: endpoint.Generation,
		})
		if statusErr := r.Status().Update(ctx, endpoint); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Build the integration definition
	def := axonops.IntegrationDefinition{
		Type:   endpoint.Spec.Type,
		Params: params,
	}

	// Set ID for update path
	if endpoint.Status.SyncedIntegrationID != "" {
		def.ID = endpoint.Status.SyncedIntegrationID
	}

	// Create or update integration
	result, err := apiClient.CreateOrUpdateIntegration(ctx, endpoint.Spec.ClusterType, endpoint.Spec.ClusterName, def)
	if err != nil {
		log.Error(err, "Failed to create/update integration")
		meta.SetStatusCondition(&endpoint.Status.Conditions, metav1.Condition{
			Type:               condTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             ReasonAPIError,
			Message:            fmt.Sprintf("Failed to create/update integration: %v", err),
			ObservedGeneration: endpoint.Generation,
		})
		if statusErr := r.Status().Update(ctx, endpoint); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Re-fetch the CR to avoid ResourceVersion conflicts
	if err := r.Get(ctx, req.NamespacedName, endpoint); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update status
	now := metav1.Now()
	endpoint.Status.SyncedIntegrationID = result.ID
	endpoint.Status.LastSyncTime = &now
	endpoint.Status.ObservedGeneration = endpoint.Generation
	meta.SetStatusCondition(&endpoint.Status.Conditions, metav1.Condition{
		Type:               condTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             "EndpointSynced",
		Message:            "Alert endpoint successfully synced with AxonOps",
		ObservedGeneration: endpoint.Generation,
	})

	if err := r.Status().Update(ctx, endpoint); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled AxonOpsAlertEndpoint", "integrationID", result.ID)
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when the CR is being deleted
func (r *AxonOpsAlertEndpointReconciler) handleDeletion(ctx context.Context, endpoint *alertsv1alpha1.AxonOpsAlertEndpoint) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(endpoint, alertEndpointFinalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Deleting alert endpoint from AxonOps",
		"integrationID", endpoint.Status.SyncedIntegrationID,
		"type", endpoint.Spec.Type,
		"name", endpoint.Spec.Name)

	// Resolve API client for deletion
	apiClient, err := ResolveAPIClient(ctx, r.Client, endpoint.Namespace, endpoint.Spec.ConnectionRef)
	if err != nil {
		log.Error(err, "Failed to resolve API client for deletion")
		// Check if non-retryable — proceed with finalizer removal
		// For connection errors we retry since the connection may come back
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Delete the integration if we have a synced ID
	if endpoint.Status.SyncedIntegrationID != "" {
		if err := apiClient.DeleteIntegration(ctx, endpoint.Spec.ClusterType, endpoint.Spec.ClusterName, endpoint.Status.SyncedIntegrationID); err != nil {
			log.Error(err, "Failed to delete integration from AxonOps", "integrationID", endpoint.Status.SyncedIntegrationID)
			var apiErr *axonops.APIError
			if errors.As(err, &apiErr) && apiErr.IsRetryable() {
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			// Non-retryable error, proceed with finalizer removal
		} else {
			log.Info("Successfully deleted integration from AxonOps API", "integrationID", endpoint.Status.SyncedIntegrationID)
		}
	}

	// Remove the finalizer
	controllerutil.RemoveFinalizer(endpoint, alertEndpointFinalizerName)
	if err := r.Update(ctx, endpoint); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted alert endpoint")
	return ctrl.Result{}, nil
}

// endpointConfigError represents a configuration validation error
type endpointConfigError struct {
	Reason    string
	Retryable bool
	Message   string
}

func (e *endpointConfigError) Error() string {
	return e.Message
}

// buildParams builds the API params map from the endpoint spec.
// It resolves SecretRef values and validates type-config consistency.
func (r *AxonOpsAlertEndpointReconciler) buildParams(ctx context.Context, endpoint *alertsv1alpha1.AxonOpsAlertEndpoint) (map[string]string, error) {
	log := logf.FromContext(ctx)
	params := make(map[string]string)
	params["name"] = endpoint.Spec.Name

	switch endpoint.Spec.Type {
	case "slack":
		if endpoint.Spec.Slack == nil {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: "spec.slack must be set when spec.type is slack",
			}
		}
		url, err := r.resolveSecretOrValue(ctx, endpoint.Namespace, endpoint.Spec.Slack.URL, endpoint.Spec.Slack.URLSecretRef)
		if err != nil {
			return nil, err
		}
		params["url"] = url
		if endpoint.Spec.Slack.Channel != "" {
			params["channel"] = endpoint.Spec.Slack.Channel
		}
		if endpoint.Spec.Slack.AxonDashURL != "" {
			params["axondashUrl"] = endpoint.Spec.Slack.AxonDashURL
		}

	case "pagerduty":
		if endpoint.Spec.PagerDuty == nil {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: "spec.pagerDuty must be set when spec.type is pagerduty",
			}
		}
		key, err := r.resolveSecretOrValue(ctx, endpoint.Namespace, endpoint.Spec.PagerDuty.IntegrationKey, endpoint.Spec.PagerDuty.IntegrationKeySecretRef)
		if err != nil {
			return nil, err
		}
		params["integration_key"] = key

	case "opsgenie":
		if endpoint.Spec.OpsGenie == nil {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: "spec.opsGenie must be set when spec.type is opsgenie",
			}
		}
		key, err := r.resolveSecretOrValue(ctx, endpoint.Namespace, endpoint.Spec.OpsGenie.OpsGenieKey, endpoint.Spec.OpsGenie.OpsGenieKeySecretRef)
		if err != nil {
			return nil, err
		}
		params["opsgenie_key"] = key

	case "servicenow":
		if endpoint.Spec.ServiceNow == nil {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: "spec.serviceNow must be set when spec.type is servicenow",
			}
		}
		params["instance_name"] = endpoint.Spec.ServiceNow.InstanceName
		params["user"] = endpoint.Spec.ServiceNow.User
		password, err := r.resolveSecretOrValue(ctx, endpoint.Namespace, endpoint.Spec.ServiceNow.Password, endpoint.Spec.ServiceNow.PasswordSecretRef)
		if err != nil {
			return nil, err
		}
		params["password"] = password

	case "microsoft_teams":
		if endpoint.Spec.MicrosoftTeams == nil {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: "spec.microsoftTeams must be set when spec.type is microsoft_teams",
			}
		}
		url, err := r.resolveSecretOrValue(ctx, endpoint.Namespace, endpoint.Spec.MicrosoftTeams.WebHookURL, endpoint.Spec.MicrosoftTeams.WebHookURLSecretRef)
		if err != nil {
			return nil, err
		}
		params["webHookURL"] = url

	default:
		// Generic types: email, smtp, webhook
		if endpoint.Spec.Params != nil {
			// Log if a typed config is also set (it will be ignored)
			if endpoint.Spec.Slack != nil || endpoint.Spec.PagerDuty != nil || endpoint.Spec.OpsGenie != nil ||
				endpoint.Spec.ServiceNow != nil || endpoint.Spec.MicrosoftTeams != nil {
				log.Info("Typed config struct is set but will be ignored for generic integration type", "type", endpoint.Spec.Type)
			}
			maps.Copy(params, endpoint.Spec.Params)
		} else {
			return nil, &endpointConfigError{
				Reason:  "InvalidConfig",
				Message: fmt.Sprintf("spec.params must be set when spec.type is %s", endpoint.Spec.Type),
			}
		}
	}

	// Log if params is set alongside a typed config (typed config takes precedence)
	if typedIntegrationTypes[endpoint.Spec.Type] && endpoint.Spec.Params != nil {
		log.Info("spec.params is set but will be ignored because typed config takes precedence", "type", endpoint.Spec.Type)
	}

	return params, nil
}

// resolveSecretOrValue resolves a value from a SecretRef or falls back to the plain-text value.
// SecretRef takes priority when both are set.
func (r *AxonOpsAlertEndpointReconciler) resolveSecretOrValue(ctx context.Context, namespace, plainValue string, secretRef *alertsv1alpha1.SecretKeyRef) (string, error) {
	if secretRef != nil {
		secret := &corev1.Secret{}
		secretKey := types.NamespacedName{Namespace: namespace, Name: secretRef.Name}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			return "", &endpointConfigError{
				Reason:    "MissingCredential",
				Retryable: true,
				Message:   fmt.Sprintf("Failed to get secret %s: %v", secretRef.Name, err),
			}
		}
		data, ok := secret.Data[secretRef.Key]
		if !ok {
			return "", &endpointConfigError{
				Reason:    "MissingCredential",
				Retryable: true,
				Message:   fmt.Sprintf("Secret %s does not contain key %q", secretRef.Name, secretRef.Key),
			}
		}
		return string(data), nil
	}
	return plainValue, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsAlertEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&alertsv1alpha1.AxonOpsAlertEndpoint{}).
		Named("alerts-axonopsalertendpoint").
		Complete(r)
}
