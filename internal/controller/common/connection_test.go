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

package common

import (
	"fmt"
	"strings"
	"testing"

	"github.com/axonops/axonops-operator/internal/axonops"
)

const (
	testProtocolHTTPS = "https"
	testProtocolHTTP  = "http"
	testOrgID         = "my-org"
	testHost          = "axonops.internal"
)

func TestResolvePort(t *testing.T) {
	tests := []struct {
		name     string
		port     *int32
		protocol string
		want     int32
	}{
		{"nil port, https", nil, testProtocolHTTPS, 443},
		{"nil port, http", nil, testProtocolHTTP, 80},
		{"nil port, empty defaults to https", nil, "", 443},
		{"explicit 8443, https", new(int32(8443)), testProtocolHTTPS, 8443},
		{"explicit 8080, http", new(int32(8080)), testProtocolHTTP, 8080},
		{"explicit 443, https", new(int32(443)), testProtocolHTTPS, 443},
		{"explicit 80, http", new(int32(80)), testProtocolHTTP, 80},
		{"explicit 1, https", new(int32(1)), testProtocolHTTPS, 1},
		{"explicit 65535, http", new(int32(65535)), testProtocolHTTP, 65535},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePort(tt.port, tt.protocol)
			if got != tt.want {
				t.Errorf("resolvePort() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBuildHostURL_WithPort(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int32
		orgID    string
		useSAML  bool
		protocol string
		want     string
	}{
		{"default host, https standard port", "", 443, testOrgID, false, testProtocolHTTPS, "https://dash.axonops.cloud/my-org"},
		{"default host, http standard port", "", 80, testOrgID, false, testProtocolHTTP, "http://dash.axonops.cloud/my-org"},
		{"custom host, non-standard https", testHost, 8443, testOrgID, false, testProtocolHTTPS, "https://axonops.internal:8443/my-org"},
		{"custom host, standard https port", testHost, 443, testOrgID, false, testProtocolHTTPS, "https://axonops.internal/my-org"},
		{"custom host, non-standard http", testHost, 8080, testOrgID, false, testProtocolHTTP, "http://axonops.internal:8080/my-org"},
		{"custom host, standard http port", testHost, 80, testOrgID, false, testProtocolHTTP, "http://axonops.internal/my-org"},
		{"SAML, non-standard port", testHost, 9443, testOrgID, true, testProtocolHTTPS, "https://axonops.internal:9443/dashboard"},
		{"SAML, standard port", testHost, 443, testOrgID, true, testProtocolHTTPS, "https://axonops.internal/dashboard"},
		{"SAML default host", "", 443, testOrgID, true, testProtocolHTTPS, "https://my-org.axonops.cloud/dashboard"},
		{"empty protocol defaults to https", testHost, 443, testOrgID, false, "", "https://axonops.internal/my-org"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildHostURL(tt.host, tt.port, tt.orgID, tt.useSAML, tt.protocol)
			if got != tt.want {
				t.Errorf("BuildHostURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSafeConditionMsg_APIError_OmitsBody(t *testing.T) {
	apiErr := &axonops.APIError{StatusCode: 500, Body: "internal server error details"}
	msg := SafeConditionMsg("Failed to sync", apiErr)

	if strings.Contains(msg, "internal server error details") {
		t.Errorf("SafeConditionMsg() must not include API response body, got: %q", msg)
	}
	if !strings.Contains(msg, "500") {
		t.Errorf("SafeConditionMsg() must include HTTP status code, got: %q", msg)
	}
	if !strings.Contains(msg, "Failed to sync") {
		t.Errorf("SafeConditionMsg() must include prefix, got: %q", msg)
	}
}

func TestSafeConditionMsg_NonAPIError_IncludesMessage(t *testing.T) {
	err := fmt.Errorf("connection refused")
	msg := SafeConditionMsg("Failed to connect", err)

	if !strings.Contains(msg, "connection refused") {
		t.Errorf("SafeConditionMsg() should include non-API error message, got: %q", msg)
	}
	if !strings.Contains(msg, "Failed to connect") {
		t.Errorf("SafeConditionMsg() must include prefix, got: %q", msg)
	}
}

func TestSafeConditionMsg_WrappedAPIError_OmitsBody(t *testing.T) {
	apiErr := &axonops.APIError{StatusCode: 403, Body: "sensitive response content"}
	wrapped := fmt.Errorf("outer context: %w", apiErr)
	msg := SafeConditionMsg("Failed to get resource", wrapped)

	if strings.Contains(msg, "sensitive response content") {
		t.Errorf("SafeConditionMsg() must not include body from wrapped APIError, got: %q", msg)
	}
	if !strings.Contains(msg, "403") {
		t.Errorf("SafeConditionMsg() must include status code from wrapped APIError, got: %q", msg)
	}
}
