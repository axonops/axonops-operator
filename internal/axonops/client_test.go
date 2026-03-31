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

package axonops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestURLPathEscaping_SpecialCharacters(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RawPath
		if receivedPath == "" {
			receivedPath = r.URL.Path
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metricrules":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "http", "org/evil", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// orgID with "/" should be escaped
	_, _ = client.GetMetricAlertRules(context.Background(), "cassandra", "normal-cluster")

	if strings.Contains(receivedPath, "org/evil") {
		t.Errorf("orgID was not escaped: path = %s", receivedPath)
	}
	if !strings.Contains(receivedPath, "org%2Fevil") {
		t.Errorf("orgID should be escaped as org%%2Fevil: path = %s", receivedPath)
	}
}

func TestURLPathEscaping_ClusterNameWithDots(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RawPath
		if receivedPath == "" {
			receivedPath = r.URL.Path
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metricrules":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "http", "myorg", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Cluster name with path traversal attempt
	_, _ = client.GetMetricAlertRules(context.Background(), "cassandra", "../../admin")

	if strings.Contains(receivedPath, "../../admin") {
		t.Errorf("clusterName path traversal was not escaped: path = %s", receivedPath)
	}
}

func TestURLPathEscaping_KafkaTopicName(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RawPath
		if receivedPath == "" {
			receivedPath = r.URL.Path
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "http", "myorg", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_ = client.DeleteKafkaTopic(context.Background(), "my-cluster", "topic/with/slashes")

	if strings.Contains(receivedPath, "topic/with/slashes") {
		t.Errorf("topic name with slashes was not escaped: path = %s", receivedPath)
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{400, false},
		{401, false},
		{404, false},
		{499, false},
		{500, true},
		{502, true},
		{503, true},
	}
	for _, tt := range tests {
		apiErr := &APIError{StatusCode: tt.code}
		if got := apiErr.IsRetryable(); got != tt.want {
			t.Errorf("APIError{StatusCode: %d}.IsRetryable() = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	c, err := NewClient("localhost", "http", "org", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	c, err := NewClient("localhost", "http", "org", "key", "Bearer", false, WithTimeout(90*time.Second))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.httpClient.Timeout != 90*time.Second {
		t.Errorf("expected timeout 90s, got %v", c.httpClient.Timeout)
	}
}

func TestNewClient_ZeroTimeout(t *testing.T) {
	c, err := NewClient("localhost", "http", "org", "key", "Bearer", false, WithTimeout(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("zero timeout should fall back to default %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}
}

func TestNewClient_NegativeTimeout(t *testing.T) {
	c, err := NewClient("localhost", "http", "org", "key", "Bearer", false, WithTimeout(-5*time.Second))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("negative timeout should fall back to default %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}
}

func TestNewClient_SAMLHostPathPreserved(t *testing.T) {
	// When a full URL with path (e.g. SAML host "https://org.axonopsdev.com/dashboard")
	// is passed as host, the /dashboard path must be preserved in baseURL.
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"metricrules":[]}`))
	}))
	defer server.Close()

	// Simulate BuildHostURL output for SAML mode: full URL with /dashboard suffix
	samlHost := server.URL + "/dashboard"
	client, err := NewClient(samlHost, "", "myorg", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, _ = client.GetMetricAlertRules(context.Background(), "cassandra", "mycluster")

	if !strings.HasPrefix(receivedPath, "/dashboard/") {
		t.Errorf("SAML /dashboard path was stripped: got path %q, want prefix /dashboard/", receivedPath)
	}
}

func TestAPIError_SafeMessage_OmitsBody(t *testing.T) {
	e := &APIError{StatusCode: 500, Body: "internal secret details"}
	safe := e.SafeMessage()
	if strings.Contains(safe, "internal secret details") {
		t.Errorf("SafeMessage() must not include response body, got: %q", safe)
	}
	if !strings.Contains(safe, "500") {
		t.Errorf("SafeMessage() must include status code, got: %q", safe)
	}
}

func TestAPIError_Error_IncludesBody(t *testing.T) {
	e := &APIError{StatusCode: 403, Body: "forbidden response body"}
	full := e.Error()
	if !strings.Contains(full, "forbidden response body") {
		t.Errorf("Error() should include body for debug logging, got: %q", full)
	}
}

func TestAPIError_SafeMessage_OmitsOrgID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"org-secret-id not found"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "http", "org-secret-id", "key", "Bearer", false)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	_, apiErr := client.GetMetricAlertRules(context.Background(), "cassandra", "cluster")
	var ae *APIError
	if !errors.As(apiErr, &ae) {
		t.Fatalf("expected *APIError, got %T", apiErr)
	}

	safe := ae.SafeMessage()
	if strings.Contains(safe, "org-secret-id") {
		t.Errorf("SafeMessage() must not contain orgID, got: %q", safe)
	}
}

// newTLSTestClient creates a Client wired to the given httptest.Server (TLS).
func newTLSTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	c, err := NewClient(server.URL, "", "myorg", "key", "Bearer", true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestCreateKafkaTopic_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"available kafka nodes not found"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).CreateKafkaTopic(context.Background(), "cluster", KafkaTopicCreateRequest{TopicName: "t"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "available kafka nodes not found") {
		t.Errorf("Body = %q, want it to contain error message", apiErr.Body)
	}
}

func TestCreateKafkaTopic_ValidBody_ReturnsNil(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).CreateKafkaTopic(context.Background(), "cluster", KafkaTopicCreateRequest{TopicName: "t"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestCreateKafkaTopic_EmptyBody_ReturnsNil(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).CreateKafkaTopic(context.Background(), "cluster", KafkaTopicCreateRequest{TopicName: "t"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestUpdateKafkaTopicConfig_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"topic not found"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).UpdateKafkaTopicConfig(context.Background(), "cluster", "topic", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
}

func TestDeleteKafkaTopic_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"topic deletion failed"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).DeleteKafkaTopic(context.Background(), "cluster", "my-topic")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
}

func TestCreateKafkaACL_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"acl creation failed"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).CreateKafkaACL(context.Background(), "cluster", KafkaACL{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "acl creation failed") {
		t.Errorf("Body = %q, want it to contain error message", apiErr.Body)
	}
}

func TestDeleteKafkaACL_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"acl not found"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).DeleteKafkaACL(context.Background(), "cluster", KafkaACL{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
}

func TestCreateKafkaConnector_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"connect cluster unavailable"}`))
	}))
	defer server.Close()

	result, err := newTLSTestClient(t, server).CreateKafkaConnector(context.Background(), "cluster", "connect", KafkaConnectorCreateRequest{})
	if result != nil {
		t.Errorf("expected nil result, got %+v", result)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "connect cluster unavailable") {
		t.Errorf("Body = %q, want it to contain error message", apiErr.Body)
	}
}

func TestCreateKafkaConnector_ValidBody_DecodesResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"my-connector","type":"sink","config":{"connector.class":"io.kafka.Sink"}}`))
	}))
	defer server.Close()

	result, err := newTLSTestClient(t, server).CreateKafkaConnector(context.Background(), "cluster", "connect", KafkaConnectorCreateRequest{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "my-connector" {
		t.Errorf("Name = %q, want %q", result.Name, "my-connector")
	}
	if result.Type != "sink" {
		t.Errorf("Type = %q, want %q", result.Type, "sink")
	}
}

func TestUpdateKafkaConnectorConfig_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"connector config update failed"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).UpdateKafkaConnectorConfig(context.Background(), "cluster", "connect", "my-connector", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
}

func TestDeleteKafkaConnector_BodyError_ReturnsAPIError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"connector not found"}`))
	}))
	defer server.Close()

	err := newTLSTestClient(t, server).DeleteKafkaConnector(context.Background(), "cluster", "connect", "my-connector")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", apiErr.StatusCode)
	}
}

func TestCheckBodyError_NonJSONBody_ReturnsNil(t *testing.T) {
	// Non-JSON body (e.g. empty DELETE response) must not cause an error
	if err := checkBodyError([]byte{}, 200); err != nil {
		t.Errorf("expected nil for empty body, got %v", err)
	}
	if err := checkBodyError([]byte("not json"), 200); err != nil {
		t.Errorf("expected nil for non-JSON body, got %v", err)
	}
}

func TestCheckBodyError_ErrorFieldOnly_StoredInBody(t *testing.T) {
	// Body field on the returned APIError must be the error string, not the full JSON
	err := checkBodyError([]byte(`{"error":"something went wrong","other":"ignored"}`), 200)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError")
	}
	if apiErr.Body != "something went wrong" {
		t.Errorf("Body = %q, want %q", apiErr.Body, "something went wrong")
	}
}

func TestNewClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		orgID   string
		apiKey  string
		wantErr bool
	}{
		{"valid", "localhost", "org", "key", false},
		{"empty host", "", "org", "key", true},
		{"empty orgID", "localhost", "", "key", true},
		{"empty apiKey", "localhost", "org", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.host, "http", tt.orgID, tt.apiKey, "Bearer", false)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
