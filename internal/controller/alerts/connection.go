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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// Condition reasons for alert and route resources
const (
	ReasonConnectionError     = "ConnectionError"
	ReasonAPIError            = "APIError"
	ReasonDashboardNotFound   = "DashboardNotFound"
	ReasonAlertSynced         = "AlertSynced"
	ReasonRouteSynced         = "RouteSynced"
	ReasonIntegrationNotFound = "IntegrationNotFound"
	ReasonInvalidRouteType    = "InvalidRouteType"
	ReasonOverrideError       = "OverrideError"
	ReasonRouteError          = "RouteError"
)

// ResolveAPIClient resolves the AxonOps API client from a referenced AxonOpsConnection.
func ResolveAPIClient(ctx context.Context, c client.Client, namespace, connectionRef string) (*axonops.Client, error) {
	log := logf.FromContext(ctx)

	// connectionRef is required for security and declarative configuration
	if connectionRef == "" {
		return nil, fmt.Errorf("connectionRef must be specified")
	}

	// Resolve from the connection resource
	conn := &corev1alpha1.AxonOpsConnection{}
	connKey := types.NamespacedName{Namespace: namespace, Name: connectionRef}
	if err := c.Get(ctx, connKey, conn); err != nil {
		return nil, fmt.Errorf("failed to get AxonOpsConnection: %w", err)
	}

	// Read the API key from the referenced secret
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Namespace: namespace, Name: conn.Spec.APIKeyRef.Name}
	if err := c.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", secretKey, err)
	}

	// Extract the API key from the secret
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

	// Build client from connection spec
	tokenType := conn.Spec.TokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}

	// Construct host URL
	fullHost := buildHostURL(conn.Spec.Host, conn.Spec.OrgID, conn.Spec.UseSAML)

	apiClient, err := axonops.NewClient(fullHost, "", conn.Spec.OrgID, string(apiKey), tokenType, conn.Spec.TLSSkipVerify)
	if err != nil {
		return nil, fmt.Errorf("failed to create AxonOps client from connection: %w", err)
	}
	log.Info("Resolved API client from AxonOpsConnection", "connection", connKey)
	return apiClient, nil
}

// buildHostURL constructs the AxonOps API host URL based on the connection settings
// This mirrors the Terraform provider's URL construction logic
func buildHostURL(customHost, orgID string, useSAML bool) string {
	var host string
	if customHost == "" {
		// No custom host - use SaaS defaults
		if useSAML {
			host = fmt.Sprintf("%s.axonops.cloud/dashboard", orgID)
		} else {
			host = fmt.Sprintf("dash.axonops.cloud/%s", orgID)
		}
	} else {
		// Custom host provided
		if useSAML {
			host = fmt.Sprintf("%s/dashboard", customHost)
		} else {
			host = fmt.Sprintf("%s/%s", customHost, orgID)
		}
	}

	// Return URL with https protocol (protocol handling is done by axonops.NewClient)
	return fmt.Sprintf("https://%s", host)
}
