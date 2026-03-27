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

package common

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// Shared condition reasons used across controller groups
const (
	ReasonConnectionError  = "ConnectionError"
	ReasonAPIError         = "APIError"
	ReasonConnectionPaused = "ConnectionPaused"
)

// ErrConnectionPaused is returned by ResolveAPIClient when the referenced
// AxonOpsConnection has spec.pause set to true. Controllers must handle this
// by setting a Paused condition and returning without error.
var ErrConnectionPaused = fmt.Errorf("AxonOpsConnection is paused")

// ResolveAPIClient resolves the AxonOps API client from a referenced AxonOpsConnection.
func ResolveAPIClient(ctx context.Context, c client.Client, namespace, connectionRef string) (*axonops.Client, error) {
	log := logf.FromContext(ctx)

	if connectionRef == "" {
		return nil, fmt.Errorf("connectionRef must be specified")
	}

	conn := &corev1alpha1.AxonOpsConnection{}
	connKey := types.NamespacedName{Namespace: namespace, Name: connectionRef}
	if err := c.Get(ctx, connKey, conn); err != nil {
		return nil, fmt.Errorf("failed to get AxonOpsConnection: %w", err)
	}

	if conn.Spec.Pause {
		return nil, ErrConnectionPaused
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: namespace, Name: conn.Spec.APIKeyRef.Name}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", secretKey, err)
	}

	keyName := conn.Spec.APIKeyRef.Key
	if keyName == "" {
		keyName = "api_key"
	}
	apiKey, ok := secret.Data[keyName]
	if !ok {
		return nil, fmt.Errorf("secret %s does not have key %q", secretKey, keyName)
	}
	if len(apiKey) == 0 {
		return nil, fmt.Errorf("secret %s key %q is empty", secretKey, keyName)
	}

	tokenType := conn.Spec.TokenType
	if tokenType == "" {
		tokenType = axonops.DefaultTokenType(conn.Spec.Host)
	}

	fullHost := BuildHostURL(conn.Spec.Host, conn.Spec.OrgID, conn.Spec.UseSAML)

	var opts []axonops.ClientOption
	if conn.Spec.Timeout != "" {
		d, err := time.ParseDuration(conn.Spec.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout %q in AxonOpsConnection: %w", conn.Spec.Timeout, err)
		}
		opts = append(opts, axonops.WithTimeout(d))
	}

	apiClient, err := axonops.NewClient(fullHost, "", conn.Spec.OrgID, string(apiKey), tokenType, conn.Spec.TLSSkipVerify, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create AxonOps client from connection: %w", err)
	}
	log.Info("Resolved API client from AxonOpsConnection", "connection", connKey)
	return apiClient, nil
}

// BuildHostURL constructs the AxonOps API host URL based on the connection settings.
func BuildHostURL(customHost, orgID string, useSAML bool) string {
	var host string
	if customHost == "" {
		if useSAML {
			host = fmt.Sprintf("%s.axonops.cloud/dashboard", orgID)
		} else {
			host = fmt.Sprintf("dash.axonops.cloud/%s", orgID)
		}
	} else {
		if useSAML {
			host = fmt.Sprintf("%s/dashboard", customHost)
		} else {
			host = fmt.Sprintf("%s/%s", customHost, orgID)
		}
	}
	return fmt.Sprintf("https://%s", host)
}

// HandleConnectionPaused sets a Paused condition on the resource and updates its status.
// Call this when ResolveAPIClient returns ErrConnectionPaused.
func HandleConnectionPaused(ctx context.Context, c client.Client, obj client.Object, conditions *[]metav1.Condition) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Connection is paused, skipping reconciliation")

	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    "Paused",
		Status:  metav1.ConditionTrue,
		Reason:  ReasonConnectionPaused,
		Message: "Referenced AxonOpsConnection is paused",
	})

	if err := c.Status().Update(ctx, obj); err != nil {
		log.Error(err, "Failed to update paused status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ClearPausedCondition removes the Paused condition if it exists.
func ClearPausedCondition(conditions *[]metav1.Condition) {
	meta.RemoveStatusCondition(conditions, "Paused")
}
