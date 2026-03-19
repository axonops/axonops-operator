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
	"context"
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
