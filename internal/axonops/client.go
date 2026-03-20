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

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultTimeout is the default HTTP client timeout for AxonOps API requests.
	DefaultTimeout = 30 * time.Second
)

// Client manages communication with the AxonOps API
type Client struct {
	httpClient *http.Client
	baseURL    string
	orgID      string
	apiKey     string
	tokenType  string
}

// ClientOption configures optional Client settings.
type ClientOption func(*clientOptions)

type clientOptions struct {
	timeout time.Duration
}

// WithTimeout sets the HTTP client timeout. Values <= 0 are ignored and the
// default (30 s) is used instead.
func WithTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.timeout = d
	}
}

// NewClient creates a new AxonOps API client.
// Optional ClientOption values can be appended to customise behaviour
// (e.g. WithTimeout).
func NewClient(host, protocol, orgID, apiKey, tokenType string, tlsSkipVerify bool, opts ...ClientOption) (*Client, error) {
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if orgID == "" {
		return nil, fmt.Errorf("orgID is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("apiKey is required")
	}
	if protocol == "" {
		protocol = "https"
	}
	if tokenType == "" {
		tokenType = "Bearer"
	}

	// Apply options
	o := &clientOptions{timeout: DefaultTimeout}
	for _, opt := range opts {
		opt(o)
	}
	if o.timeout <= 0 {
		o.timeout = DefaultTimeout
	}

	// Ensure host is properly formatted (remove protocol if included)
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		u, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("invalid host URL: %w", err)
		}
		host = u.Host
	}

	baseURL := fmt.Sprintf("%s://%s", protocol, host)

	httpClient := &http.Client{
		Timeout: o.timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: tlsSkipVerify,
			},
		},
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		orgID:      orgID,
		apiKey:     apiKey,
		tokenType:  tokenType,
	}, nil
}

// GetMetricAlertRules fetches all metric alert rules for a cluster
func (c *Client) GetMetricAlertRules(ctx context.Context, clusterType, clusterName string) ([]MetricAlertRule, error) {
	reqURL := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	// URL format: https://host/api/v1/alert-rules/{orgId}/{clusterType}/{clusterName}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get alert rules: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result struct {
		MetricRules []MetricAlertRule `json:"metricrules"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.MetricRules, nil
}

// CreateOrUpdateMetricAlertRule creates or updates a metric alert rule.
// The server determines create vs update solely from the `id` field in the POST body:
// if the id already exists it updates, otherwise it creates. A UUID is generated
// client-side when no ID is present so the caller can track it for future updates.
func (c *Client) CreateOrUpdateMetricAlertRule(ctx context.Context, clusterType, clusterName string, rule MetricAlertRule) (MetricAlertRule, error) {
	// Always POST to the base URL — the id in the body is what the server uses.
	reqURL := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	// Generate a stable UUID when no ID exists so the server treats this as a specific
	// new record rather than generating its own, allowing us to detect duplicates later.
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}

	body, err := json.Marshal(rule)
	if err != nil {
		return MetricAlertRule{}, fmt.Errorf("failed to marshal rule: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return MetricAlertRule{}, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MetricAlertRule{}, fmt.Errorf("failed to create/update alert rule: %w", err)
	}
	defer resp.Body.Close()

	// 200, 201, 204 are all success codes
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return MetricAlertRule{}, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	// 204 returns no content, so return the original rule with ID unchanged
	if resp.StatusCode == http.StatusNoContent {
		return rule, nil
	}

	var result MetricAlertRule
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return MetricAlertRule{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return result, nil
}

// DeleteMetricAlertRule deletes a metric alert rule
func (c *Client) DeleteMetricAlertRule(ctx context.Context, clusterType, clusterName, alertID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName), p(alertID))
	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete alert rule: %w", err)
	}
	defer resp.Body.Close()

	// 200 and 204 are success codes
	// 404 is also acceptable (already deleted)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return nil
}

// ResolveDashboardPanel resolves a dashboard and chart name to a panel UUID (correlation ID)
func (c *Client) ResolveDashboardPanel(ctx context.Context, clusterType, clusterName, dashboardName, chartTitle string) (string, error) {
	reqURL := fmt.Sprintf("%s/api/v1/dashboardtemplate/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	// URL format: https://host/api/v1/dashboardtemplate/{orgId}/{clusterType}/{clusterName}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to resolve dashboard: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	// Read body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result DashboardTemplateResponse
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode dashboard response: %w", err)
	}

	// Find the dashboard and chart
	for _, dashboard := range result.Dashboards {
		if dashboard.Name == dashboardName {
			for _, panel := range dashboard.Panels {
				if panel.Title == chartTitle {
					return panel.UUID, nil
				}
			}
		}
	}

	return "", fmt.Errorf("dashboard %q with chart %q not found", dashboardName, chartTitle)
}

// GetDashboardTemplates fetches all dashboard templates for a cluster using API v2.0
func (c *Client) GetDashboardTemplates(ctx context.Context, clusterType, clusterName string) (*DashboardTemplateGetResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/dashboardtemplate/%s/%s/%s?dashver=2.0", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboard templates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result DashboardTemplateGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode dashboard templates response: %w", err)
	}

	return &result, nil
}

// UpdateDashboardTemplates replaces all dashboard templates for a cluster using API v2.0.
// The full list of dashboards must be provided — the API replaces all dashboards on PUT.
func (c *Client) UpdateDashboardTemplates(ctx context.Context, clusterType, clusterName string, payload DashboardTemplatePutPayload) error {
	reqURL := fmt.Sprintf("%s/api/v1/dashboardtemplate/%s/%s/%s?dashver=2.0", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal dashboard templates: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update dashboard templates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// setAuthHeader sets the authorization header on the request
func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", c.tokenType, c.apiKey))
}

// p escapes a string for safe use as a URL path segment.
// Prevents path traversal via user-controlled CRD field values.
func p(s string) string {
	return url.PathEscape(s)
}

// APIError represents an error from the AxonOps API
type APIError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface
func (e *APIError) Error() string {
	return fmt.Sprintf("AxonOps API error (status %d): %s", e.StatusCode, e.Body)
}

// IsRetryable returns true if the error is retryable (server error)
func (e *APIError) IsRetryable() bool {
	return e.StatusCode >= 500
}

// GetIntegrations retrieves all integrations and their routing configurations for a cluster
func (c *Client) GetIntegrations(ctx context.Context, clusterType, clusterName string) (*IntegrationsResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/integrations/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get integrations: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result IntegrationsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// AddIntegrationRoute adds an alert route to an integration
func (c *Client) AddIntegrationRoute(ctx context.Context, clusterType, clusterName, routeType, severity, integrationID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/integrations-routing/%s/%s/%s/%s/%s/%s",
		c.baseURL, p(c.orgID), p(clusterType), p(clusterName), routeType, p(severity), p(integrationID))

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add integration route: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return nil
}

// RemoveIntegrationRoute removes an alert route from an integration
func (c *Client) RemoveIntegrationRoute(ctx context.Context, clusterType, clusterName, routeType, severity, integrationID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/integrations-routing/%s/%s/%s/%s/%s/%s",
		c.baseURL, p(c.orgID), p(clusterType), p(clusterName), routeType, p(severity), p(integrationID))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove integration route: %w", err)
	}
	defer resp.Body.Close()

	// 200, 204, and 404 are success (404 means already deleted)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return nil
}

// SetIntegrationOverride sets the override flag for a route type and severity
func (c *Client) SetIntegrationOverride(ctx context.Context, clusterType, clusterName, routeType, severity string, value bool) error {
	reqURL := fmt.Sprintf("%s/api/v1/integrations-override/%s/%s/%s/%s/%s",
		c.baseURL, p(c.orgID), p(clusterType), p(clusterName), routeType, p(severity))

	payload := map[string]bool{"value": value}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set integration override: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// GetHealthchecks retrieves all healthchecks for a cluster
func (c *Client) GetHealthchecks(ctx context.Context, clusterType, clusterName string) (*HealthchecksResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/healthchecks/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get healthchecks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result HealthchecksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// UpdateHealthchecks updates all healthchecks for a cluster (bulk update)
// This replaces all healthchecks with the provided list
func (c *Client) UpdateHealthchecks(ctx context.Context, clusterType, clusterName string, healthchecks *HealthchecksResponse) error {
	reqURL := fmt.Sprintf("%s/api/v1/healthchecks/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal(healthchecks)
	if err != nil {
		return fmt.Errorf("failed to marshal healthchecks: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update healthchecks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// CreateOrUpdateIntegration creates or updates an integration definition.
// The server determines create vs update from the ID field in the POST body.
// A UUID is generated client-side when no ID is present.
func (c *Client) CreateOrUpdateIntegration(ctx context.Context, clusterType, clusterName string, def IntegrationDefinition) (IntegrationDefinition, error) {
	reqURL := fmt.Sprintf("%s/api/v1/integrations/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	// Generate a client-side UUID when no ID exists
	if def.ID == "" {
		def.ID = uuid.New().String()
	}

	// Build the request payload matching the API format
	payload := map[string]any{
		"type":   def.Type,
		"params": def.Params,
		"id":     def.ID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return IntegrationDefinition{}, fmt.Errorf("failed to marshal integration: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return IntegrationDefinition{}, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return IntegrationDefinition{}, fmt.Errorf("failed to create/update integration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return IntegrationDefinition{}, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	// 204 returns no content, return the definition with the ID we sent
	if resp.StatusCode == http.StatusNoContent {
		return def, nil
	}

	var result IntegrationDefinition
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// If we can't decode, return the definition with the ID we generated
		return def, nil
	}

	return result, nil
}

// GetCommitlogArchiveSettings fetches commitlog archive settings for a cluster
func (c *Client) GetCommitlogArchiveSettings(ctx context.Context, clusterType, clusterName string) ([]CommitlogArchiveSettings, error) {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraCommitLogsSettings/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get commitlog archive settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result []CommitlogArchiveSettings
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode commitlog archive response: %w", err)
	}

	return result, nil
}

// CreateCommitlogArchiveSettings creates or updates commitlog archive settings
func (c *Client) CreateCommitlogArchiveSettings(ctx context.Context, clusterType, clusterName string, payload CommitlogArchivePayload) error {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraCommitLogsSettings/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal commitlog archive payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create commitlog archive settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return nil
}

// DeleteCommitlogArchiveSettings deletes commitlog archive settings for a remoteType
func (c *Client) DeleteCommitlogArchiveSettings(ctx context.Context, clusterType, clusterName string, datacenters []string) error {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraCommitLogsSettings/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	payload := CommitlogArchiveDeletePayload{Datacenters: datacenters}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal delete payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete commitlog archive settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	return nil
}

// GetAdaptiveRepair fetches the current adaptive repair settings for a cluster
func (c *Client) GetAdaptiveRepair(ctx context.Context, clusterType, clusterName string) (*AdaptiveRepairSettings, error) {
	reqURL := fmt.Sprintf("%s/api/v1/adaptiveRepair/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get adaptive repair settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result AdaptiveRepairSettings
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode adaptive repair response: %w", err)
	}

	return &result, nil
}

// UpdateAdaptiveRepair updates the adaptive repair settings for a cluster
func (c *Client) UpdateAdaptiveRepair(ctx context.Context, clusterType, clusterName string, settings AdaptiveRepairSettings) error {
	reqURL := fmt.Sprintf("%s/api/v1/adaptiveRepair/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal adaptive repair settings: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update adaptive repair settings: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// DeleteIntegration deletes an integration by ID.
// Returns nil on 200, 204, or 404 (already deleted).
func (c *Client) DeleteIntegration(ctx context.Context, clusterType, clusterName, integrationID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/integrations/%s/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName), p(integrationID))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete integration: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// GetScheduledSnapshots retrieves all scheduled snapshots for a cluster
func (c *Client) GetScheduledSnapshots(ctx context.Context, clusterType, clusterName string) (*ScheduledSnapshotResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraScheduleSnapshot/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled snapshots: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var result ScheduledSnapshotResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// CreateScheduledSnapshot creates a new scheduled snapshot.
// A UUID is generated client-side when no ID is present.
func (c *Client) CreateScheduledSnapshot(ctx context.Context, clusterType, clusterName string, payload BackupPayload) error {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraSnapshot/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	if payload.ID == "" {
		payload.ID = uuid.New().String()
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create scheduled snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// DeleteScheduledSnapshot deletes a scheduled snapshot by ID.
// The API expects a JSON array of IDs in the request body.
func (c *Client) DeleteScheduledSnapshot(ctx context.Context, clusterType, clusterName, snapshotID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/cassandraScheduleSnapshot/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal([]string{snapshotID})
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot ID: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// GetScheduledRepairs retrieves all scheduled repairs for a cluster
func (c *Client) GetScheduledRepairs(ctx context.Context, clusterType, clusterName string) (*ScheduledRepairsResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/repair/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled repairs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result ScheduledRepairsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// CreateScheduledRepair creates a new scheduled repair
func (c *Client) CreateScheduledRepair(ctx context.Context, clusterType, clusterName string, params ScheduledRepairParams) error {
	reqURL := fmt.Sprintf("%s/api/v1/addrepair/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create scheduled repair: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// DeleteScheduledRepair deletes a scheduled repair by ID (passed as query parameter)
func (c *Client) DeleteScheduledRepair(ctx context.Context, clusterType, clusterName, repairID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/cassandrascheduledrepair/%s/%s/%s?id=%s",
		c.baseURL, p(c.orgID), p(clusterType), p(clusterName), p(repairID))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled repair: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// CreateKafkaTopic creates a Kafka topic
func (c *Client) CreateKafkaTopic(ctx context.Context, clusterName string, topic KafkaTopicCreateRequest) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/topics", c.baseURL, p(c.orgID), p(clusterName))

	body, err := json.Marshal(topic)
	if err != nil {
		return fmt.Errorf("failed to marshal topic: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create kafka topic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// UpdateKafkaTopicConfig updates a Kafka topic's configuration
func (c *Client) UpdateKafkaTopicConfig(ctx context.Context, clusterName, topicName string, configs []KafkaTopicConfigUpdate) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/topics/%s/configs", c.baseURL, p(c.orgID), p(clusterName), p(topicName))

	payload := struct {
		Configs []KafkaTopicConfigUpdate `json:"configs"`
	}{Configs: configs}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal config update: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update kafka topic config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// DeleteKafkaTopic deletes a Kafka topic
func (c *Client) DeleteKafkaTopic(ctx context.Context, clusterName, topicName string) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/topics/%s", c.baseURL, p(c.orgID), p(clusterName), p(topicName))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete kafka topic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// CreateKafkaACL creates a Kafka ACL entry
func (c *Client) CreateKafkaACL(ctx context.Context, clusterName string, acl KafkaACL) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/acls", c.baseURL, p(c.orgID), p(clusterName))

	body, err := json.Marshal(acl)
	if err != nil {
		return fmt.Errorf("failed to marshal ACL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create kafka ACL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// DeleteKafkaACL deletes a Kafka ACL entry. The full ACL struct is sent as the request body.
func (c *Client) DeleteKafkaACL(ctx context.Context, clusterName string, acl KafkaACL) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/acls", c.baseURL, p(c.orgID), p(clusterName))

	body, err := json.Marshal(acl)
	if err != nil {
		return fmt.Errorf("failed to marshal ACL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete kafka ACL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// CreateKafkaConnector creates a Kafka Connect connector
func (c *Client) CreateKafkaConnector(ctx context.Context, clusterName, connectClusterName string, connector KafkaConnectorCreateRequest) (*KafkaConnectorResponse, error) {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/connect/%s/connector",
		c.baseURL, p(c.orgID), p(clusterName), p(connectClusterName))

	body, err := json.Marshal(connector)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal connector: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka connector: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}

	var result KafkaConnectorResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// UpdateKafkaConnectorConfig updates a Kafka Connect connector's configuration
func (c *Client) UpdateKafkaConnectorConfig(ctx context.Context, clusterName, connectClusterName, connectorName string, config map[string]string) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/connect/%s/%s/config",
		c.baseURL, p(c.orgID), p(clusterName), p(connectClusterName), p(connectorName))

	payload := KafkaConnectorConfigUpdate{Config: config}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update kafka connector config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// DeleteKafkaConnector deletes a Kafka Connect connector
func (c *Client) DeleteKafkaConnector(ctx context.Context, clusterName, connectClusterName, connectorName string) error {
	reqURL := fmt.Sprintf("%s/api/v1/%s/kafka/%s/connect/%s/%s",
		c.baseURL, p(c.orgID), p(clusterName), p(connectClusterName), p(connectorName))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete kafka connector: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// GetLogCollectors retrieves all log collectors for a cluster
func (c *Client) GetLogCollectors(ctx context.Context, clusterType, clusterName string) ([]LogCollector, error) {
	reqURL := fmt.Sprintf("%s/api/v1/logcollectors/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get log collectors: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result []LogCollector
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// PutLogCollectors replaces the full list of log collectors for a cluster.
// The AxonOps API expects a form-encoded body with field "addlogs".
func (c *Client) PutLogCollectors(ctx context.Context, clusterType, clusterName string, collectors []LogCollector) error {
	reqURL := fmt.Sprintf("%s/api/v1/logcollectors/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	jsonData, err := json.Marshal(collectors)
	if err != nil {
		return fmt.Errorf("failed to marshal log collectors: %w", err)
	}

	formData := url.Values{}
	formData.Set("addlogs", string(jsonData))

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to put log collectors: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// GetSilenceWindows retrieves all silence windows for a cluster
func (c *Client) GetSilenceWindows(ctx context.Context, clusterType, clusterName string) ([]SilenceWindow, error) {
	reqURL := fmt.Sprintf("%s/api/v1/silenceWindow/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get silence windows: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var result []SilenceWindow
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}

// CreateSilenceWindow creates a silence window
func (c *Client) CreateSilenceWindow(ctx context.Context, clusterType, clusterName string, silence SilenceWindow) error {
	reqURL := fmt.Sprintf("%s/api/v1/silenceWindow/%s/%s/%s", c.baseURL, p(c.orgID), p(clusterType), p(clusterName))

	if silence.ID == "" {
		silence.ID = uuid.New().String()
	}

	body, err := json.Marshal(silence)
	if err != nil {
		return fmt.Errorf("failed to marshal silence: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create silence window: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}

// DeleteSilenceWindow deletes a silence window by ID
func (c *Client) DeleteSilenceWindow(ctx context.Context, clusterType, clusterName, silenceID string) error {
	reqURL := fmt.Sprintf("%s/api/v1/silenceWindow/%s/%s/%s/%s",
		c.baseURL, p(c.orgID), p(clusterType), p(clusterName), p(silenceID))

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete silence window: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
	}
	return nil
}
