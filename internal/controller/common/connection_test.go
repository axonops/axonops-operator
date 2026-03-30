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
