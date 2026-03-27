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

import "testing"

func TestToDNSLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"High Read Latency", "high-read-latency"},
		{"io-wait", "io-wait"},
		{"CPU_Usage_Alert", "cpu-usage-alert"},
		{"  spaces  ", "spaces"},
		{"UPPER", "upper"},
		{"a.b.c", "a-b-c"},
		{"a/b/c", "a-b-c"},
		{"--leading--trailing--", "leading-trailing"},
		{"special!@#$chars", "specialchars"},
		{"", "unnamed"},
		{
			"a very long name that exceeds sixty three characters limit for dns labels in kubernetes",
			"a-very-long-name-that-exceeds-sixty-three-characters-limit-for",
		},
		{"trailing-dash-at-63-chars--------------------------------------------x", "trailing-dash-at-63-chars-x"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toDNSLabel(tt.input)
			if got != tt.expected {
				t.Errorf("toDNSLabel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
			// Verify determinism
			if got2 := toDNSLabel(tt.input); got2 != got {
				t.Errorf("toDNSLabel(%q) not deterministic: %q != %q", tt.input, got, got2)
			}
		})
	}
}
