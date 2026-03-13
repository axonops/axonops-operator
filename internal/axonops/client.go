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

// Client manages communication with the AxonOps API
type Client struct {
	httpClient *http.Client
	baseURL    string
	orgID      string
	apiKey     string
	tokenType  string
}

// NewClient creates a new AxonOps API client
func NewClient(host, protocol, orgID, apiKey, tokenType string, tlsSkipVerify bool) (*Client, error) {
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
		Timeout: 30 * time.Second,
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
	url := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s", c.baseURL, c.orgID, clusterType, clusterName)
	// URL format: https://host/api/v1/alert-rules/{orgId}/{clusterType}/{clusterName}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
	url := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s", c.baseURL, c.orgID, clusterType, clusterName)

	// Generate a stable UUID when no ID exists so the server treats this as a specific
	// new record rather than generating its own, allowing us to detect duplicates later.
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}

	body, err := json.Marshal(rule)
	if err != nil {
		return MetricAlertRule{}, fmt.Errorf("failed to marshal rule: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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
	url := fmt.Sprintf("%s/api/v1/alert-rules/%s/%s/%s/%s", c.baseURL, c.orgID, clusterType, clusterName, alertID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
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
	url := fmt.Sprintf("%s/api/v1/dashboardtemplate/%s/%s/%s", c.baseURL, c.orgID, clusterType, clusterName)
	// URL format: https://host/api/v1/dashboardtemplate/{orgId}/{clusterType}/{clusterName}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
		// Return detailed error showing the actual response
		return "", fmt.Errorf("failed to decode dashboard response (URL: %s): %w. Response body: %s", url, err, string(bodyBytes[:min(len(bodyBytes), 500)]))
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

// setAuthHeader sets the authorization header on the request
func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", c.tokenType, c.apiKey))
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
