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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/axonops/axonops-operator/internal/axonops"
	"github.com/axonops/axonops-operator/internal/controller/common"
)

// Condition reasons shared across controller groups (re-exported from common)
const (
	ReasonConnectionError = common.ReasonConnectionError
	ReasonAPIError        = common.ReasonAPIError
)

// Condition reasons specific to alert and route resources
const (
	ReasonDashboardNotFound   = "DashboardNotFound"
	ReasonAlertSynced         = "AlertSynced"
	ReasonRouteSynced         = "RouteSynced"
	ReasonIntegrationNotFound = "IntegrationNotFound"
	ReasonInvalidRouteType    = "InvalidRouteType"
	ReasonOverrideError       = "OverrideError"
	ReasonRouteError          = "RouteError"
)

// ResolveAPIClient resolves the AxonOps API client from a referenced AxonOpsConnection.
// Delegates to the shared common package implementation.
func ResolveAPIClient(ctx context.Context, c client.Client, namespace, connectionRef string) (*axonops.Client, error) {
	return common.ResolveAPIClient(ctx, c, namespace, connectionRef)
}
