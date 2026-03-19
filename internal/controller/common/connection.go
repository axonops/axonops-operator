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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// Shared condition reasons used across controller groups
const (
	ReasonConnectionError = "ConnectionError"
	ReasonAPIError        = "APIError"
)

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
		tokenType = "Bearer"
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
