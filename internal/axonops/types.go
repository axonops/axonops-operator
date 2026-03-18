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

package axonops

// MetricAlertRule represents an alert rule in AxonOps
type MetricAlertRule struct {
	ID            string                  `json:"id"` // always serialised; server uses it for create-vs-update
	IsWidget      bool                    `json:"isWidget,omitempty"`
	Alert         string                  `json:"alert"` // rule name
	For           string                  `json:"for"`   // duration
	Operator      string                  `json:"operator"`
	WarningValue  float64                 `json:"warningValue"`
	CriticalValue float64                 `json:"criticalValue"`
	Expr          string                  `json:"expr"` // full expression: metric op value
	WidgetTitle   string                  `json:"widgetTitle,omitempty"`
	CorrelationId string                  `json:"correlationId,omitempty"`
	Annotations   MetricAlertAnnotations  `json:"annotations,omitempty"`
	Filters       []MetricAlertFilter     `json:"filters,omitempty"`
	Integrations  MetricAlertIntegrations `json:"integrations,omitempty"`
}

// MetricAlertAnnotations represents alert annotations
type MetricAlertAnnotations struct {
	Description string `json:"description,omitempty"`
	Summary     string `json:"summary,omitempty"`
	WidgetUrl   string `json:"widget_url,omitempty"`
}

// MetricAlertFilter represents a single filter constraint
type MetricAlertFilter struct {
	Name  string   `json:"Name"`
	Value []string `json:"Value"`
}

// MetricAlertIntegrations represents alert notification integrations
type MetricAlertIntegrations struct {
	Type            string   `json:"Type,omitempty"`
	Routing         []string `json:"Routing,omitempty"`
	OverrideInfo    bool     `json:"OverrideInfo,omitempty"`
	OverrideWarning bool     `json:"OverrideWarning,omitempty"`
	OverrideError   bool     `json:"OverrideError,omitempty"`
}

// DashboardTemplateResponse represents the response from the dashboard template API
type DashboardTemplateResponse struct {
	Dashboards []Dashboard `json:"dashboards"`
}

// Dashboard represents a dashboard in the template response
type Dashboard struct {
	UUID   string           `json:"uuid"`
	Name   string           `json:"name"`
	Panels []DashboardPanel `json:"panels"`
}

// DashboardPanel represents a panel/chart in a dashboard
type DashboardPanel struct {
	UUID    string                `json:"uuid"`
	Title   string                `json:"title"`
	Type    string                `json:"type,omitempty"`
	Details DashboardPanelDetails `json:"details,omitempty"`
}

// DashboardPanelDetails represents the details of a dashboard panel
type DashboardPanelDetails struct {
	Queries []DashboardPanelQuery `json:"queries,omitempty"`
}

// DashboardPanelQuery represents a metric query in a panel
type DashboardPanelQuery struct {
	Query string `json:"query"`
}

// IntegrationsResponse represents the response from the integrations API
type IntegrationsResponse struct {
	Definitions []IntegrationDefinition `json:"Definitions"`
	Routings    []IntegrationRouting    `json:"Routings"`
}

// IntegrationDefinition represents an integration configuration
type IntegrationDefinition struct {
	ID     string            `json:"ID"`
	Type   string            `json:"Type"`
	Params map[string]string `json:"Params"`
}

// IntegrationRouting represents routing configuration for a route type
type IntegrationRouting struct {
	Type            string             `json:"Type"`
	Routing         []IntegrationRoute `json:"Routing"`
	OverrideInfo    bool               `json:"OverrideInfo"`
	OverrideWarning bool               `json:"OverrideWarning"`
	OverrideError   bool               `json:"OverrideError"`
}

// IntegrationRoute represents a single alert route to an integration
type IntegrationRoute struct {
	ID       string `json:"ID"`
	Severity string `json:"Severity"`
}

// HealthchecksResponse represents the response from the healthchecks API
type HealthchecksResponse struct {
	ShellChecks []HealthcheckShell `json:"shellchecks"`
	HTTPChecks  []HealthcheckHTTP  `json:"httpchecks"`
	TCPChecks   []HealthcheckTCP   `json:"tcpchecks"`
}

// HealthcheckHTTP represents an HTTP healthcheck
type HealthcheckHTTP struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	HTTP                string                  `json:"http"`
	Method              string                  `json:"method,omitempty"`
	Headers             []string                `json:"headers,omitempty"`
	Body                string                  `json:"body,omitempty"`
	ExpectedStatus      int                     `json:"expectedStatus,omitempty"`
	Interval            string                  `json:"interval,omitempty"`
	Timeout             string                  `json:"timeout,omitempty"`
	Readonly            bool                    `json:"readonly,omitempty"`
	TLSSkipVerify       bool                    `json:"tls_skip_verify,omitempty"`
	SupportedAgentTypes []string                `json:"supportedAgentTypes,omitempty"`
	Integrations        HealthcheckIntegrations `json:"integrations,omitempty"`
}

// HealthcheckTCP represents a TCP healthcheck
type HealthcheckTCP struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	TCP                 string                  `json:"tcp"`
	Interval            string                  `json:"interval,omitempty"`
	Timeout             string                  `json:"timeout,omitempty"`
	Readonly            bool                    `json:"readonly,omitempty"`
	SupportedAgentTypes []string                `json:"supportedAgentTypes,omitempty"`
	Integrations        HealthcheckIntegrations `json:"integrations,omitempty"`
}

// HealthcheckShell represents a shell healthcheck
type HealthcheckShell struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Script       string                  `json:"script"`
	Shell        string                  `json:"shell,omitempty"`
	Interval     string                  `json:"interval,omitempty"`
	Timeout      string                  `json:"timeout,omitempty"`
	Readonly     bool                    `json:"readonly,omitempty"`
	Integrations HealthcheckIntegrations `json:"integrations,omitempty"`
}

// HealthcheckIntegrations represents notification integrations for healthchecks
type HealthcheckIntegrations struct {
	Type            string   `json:"Type,omitempty"`
	Routing         []string `json:"Routing,omitempty"`
	OverrideInfo    bool     `json:"OverrideInfo,omitempty"`
	OverrideWarning bool     `json:"OverrideWarning,omitempty"`
	OverrideError   bool     `json:"OverrideError,omitempty"`
}

// AdaptiveRepairSettings represents the adaptive repair configuration for a cluster
type AdaptiveRepairSettings struct {
	Active              bool     `json:"Active"`
	BlacklistedTables   []string `json:"BlacklistedTables"`
	FilterTWCSTables    bool     `json:"FilterTWCSTables"`
	GcGraceThreshold    int64    `json:"GcGraceThreshold"`
	MaxSegmentsPerTable int32    `json:"MaxSegmentsPerTable"`
	SegmentRetries      int32    `json:"SegmentRetries"`
	SegmentTargetSizeMB int32    `json:"SegmentTargetSizeMB"`
	SegmentTimeout      string   `json:"SegmentTimeout"`
	TableParallelism    int32    `json:"TableParallelism"`
}
