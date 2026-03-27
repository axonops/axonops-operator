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

package export

import (
	"context"
	"fmt"
	"maps"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

const (
	intTypeMicrosoftTeams = "microsoft_teams"
	intTypePagerDuty      = "pagerduty"
)

// exportIntegrations fetches integration definitions and routing and converts them
// to AxonOpsAlertEndpoint and AxonOpsAlertRoute CRD resources.
func exportIntegrations(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	integrations, err := client.GetIntegrations(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching integrations: %w", err)
	}

	// Build lookup maps for route generation: ID → name and ID → type
	idToName := make(map[string]string, len(integrations.Definitions))
	idToType := make(map[string]string, len(integrations.Definitions))

	var resources []Resource

	// Export endpoint definitions
	for _, def := range integrations.Definitions {
		r := integrationDefToResource(def, opts)
		resources = append(resources, r)
		name := def.Params["name"]
		if name == "" {
			name = def.ID
		}
		idToName[def.ID] = name
		idToType[def.ID] = strings.ToLower(def.Type)
	}

	// Export routing rules
	for _, routing := range integrations.Routings {
		for _, route := range routing.Routing {
			r := integrationRouteToResource(routing, route, idToName, idToType, opts)
			if r != nil {
				resources = append(resources, *r)
			}
		}
	}

	return resources, nil
}

// integrationDefToResource converts an IntegrationDefinition to an AxonOpsAlertEndpoint resource.
func integrationDefToResource(def axonops.IntegrationDefinition, opts *Options) Resource {
	// Normalise the type to match CRD enum values
	intType := strings.ToLower(def.Type)
	// The API returns types like "Slack", "PagerDuty", "MicrosoftTeams" — normalise
	switch intType {
	case "microsoftteams":
		intType = intTypeMicrosoftTeams
	case intTypePagerDuty:
		// already correct
	}

	endpointName := def.Params["name"]
	if endpointName == "" {
		endpointName = def.ID
	}
	name := toDNSLabel(endpointName)

	obj := &alertsv1alpha1.AxonOpsAlertEndpoint{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsAlertEndpoint",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsAlertEndpointSpec{
			ConnectionRef: opts.ConnectionName,
			ClusterName:   opts.ClusterName,
			ClusterType:   opts.ClusterType,
			Name:          endpointName,
			Type:          intType,
		},
	}

	// Set type-specific config with placeholder credentials
	switch intType {
	case "slack":
		obj.Spec.Slack = &alertsv1alpha1.SlackEndpointConfig{
			URL:         "<REPLACE_WITH_WEBHOOK_URL>",
			Channel:     def.Params["channel"],
			AxonDashURL: def.Params["axondash_url"],
		}
	case intTypePagerDuty:
		obj.Spec.PagerDuty = &alertsv1alpha1.PagerDutyEndpointConfig{
			IntegrationKey: "<REPLACE_WITH_INTEGRATION_KEY>",
		}
	case "opsgenie":
		obj.Spec.OpsGenie = &alertsv1alpha1.OpsGenieEndpointConfig{
			OpsGenieKey: "<REPLACE_WITH_OPSGENIE_KEY>",
		}
	case "servicenow":
		obj.Spec.ServiceNow = &alertsv1alpha1.ServiceNowEndpointConfig{
			InstanceName: def.Params["instance"],
			User:         def.Params["user"],
			Password:     "<REPLACE_WITH_PASSWORD>",
		}
	case intTypeMicrosoftTeams:
		obj.Spec.MicrosoftTeams = &alertsv1alpha1.MicrosoftTeamsEndpointConfig{
			WebHookURL: "<REPLACE_WITH_WEBHOOK_URL>",
		}
	default:
		// email, smtp, webhook — copy params directly (no secrets exposed)
		params := make(map[string]string, len(def.Params))
		maps.Copy(params, def.Params)
		if len(params) > 0 {
			obj.Spec.Params = params
		}
	}

	return Resource{Kind: "AxonOpsAlertEndpoint", Name: name, Object: obj}
}

// integrationRouteToResource converts an integration routing entry to an AxonOpsAlertRoute.
// Returns nil when the route type or severity is not representable in the CRD.
func integrationRouteToResource(
	routing axonops.IntegrationRouting,
	route axonops.IntegrationRoute,
	idToName map[string]string,
	idToType map[string]string,
	opts *Options,
) *Resource {
	routeType := strings.ToLower(routing.Type)
	severity := strings.ToLower(route.Severity)

	// Normalise route type to CRD enum values
	switch routeType {
	case "service checks":
		routeType = "servicechecks"
	case "rolling restart":
		routeType = "rollingrestart"
	}

	validTypes := map[string]bool{
		"global": true, "metrics": true, "backups": true, "servicechecks": true,
		"nodes": true, "commands": true, "repairs": true, "rollingrestart": true,
	}
	validSeverities := map[string]bool{"info": true, "warning": true, "error": true}

	if !validTypes[routeType] || !validSeverities[severity] {
		return nil
	}

	integrationName := idToName[route.ID]
	if integrationName == "" {
		integrationName = route.ID
	}

	// Resolve the actual integration type (slack, pagerduty, etc.) from the definition map
	integrationType := idToType[route.ID]
	if integrationType == "" {
		integrationType = "email" // safe fallback
	}
	// Normalise to CRD enum
	switch integrationType {
	case intTypeMicrosoftTeams, "microsoftteams":
		integrationType = "teams"
	}

	name := toDNSLabel(fmt.Sprintf("%s-%s-%s", routeType, severity, integrationName))

	enableOverride := true
	obj := &alertsv1alpha1.AxonOpsAlertRoute{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsAlertRoute",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsAlertRouteSpec{
			ConnectionRef:   opts.ConnectionName,
			ClusterName:     opts.ClusterName,
			ClusterType:     opts.ClusterType,
			IntegrationName: integrationName,
			IntegrationType: integrationType,
			Type:            routeType,
			Severity:        severity,
			EnableOverride:  &enableOverride,
		},
	}

	r := Resource{Kind: "AxonOpsAlertRoute", Name: name, Object: obj}
	return &r
}
