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

package controller

import (
	"testing"
)

func TestConfigDataHash_DeterministicOrder(t *testing.T) {
	data := map[string][]byte{
		"z-key": []byte("z-value"),
		"a-key": []byte("a-value"),
		"m-key": []byte("m-value"),
	}

	first := configDataHash(data)
	for i := range 100 {
		got := configDataHash(data)
		if got != first {
			t.Fatalf("configDataHash produced different results: %q vs %q (iteration %d)", first, got, i)
		}
	}
}

func TestConfigDataHash_DifferentContent_DifferentHash(t *testing.T) {
	data1 := map[string][]byte{"key": []byte("value1")}
	data2 := map[string][]byte{"key": []byte("value2")}

	h1 := configDataHash(data1)
	h2 := configDataHash(data2)

	if h1 == h2 {
		t.Fatalf("expected different hashes for different content, got %q", h1)
	}
}

func TestConfigDataHash_DifferentKey_DifferentHash(t *testing.T) {
	data1 := map[string][]byte{"key1": []byte("value")}
	data2 := map[string][]byte{"key2": []byte("value")}

	h1 := configDataHash(data1)
	h2 := configDataHash(data2)

	if h1 == h2 {
		t.Fatalf("expected different hashes for different keys, got %q", h1)
	}
}

func TestConfigDataHash_NilMap_ReturnsEmpty(t *testing.T) {
	got := configDataHash(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil input, got %q", got)
	}
}

func TestConfigDataHash_EmptyMap_ReturnsEmpty(t *testing.T) {
	got := configDataHash(map[string][]byte{})
	if got != "" {
		t.Fatalf("expected empty string for empty input, got %q", got)
	}
}

func TestConfigStringDataHash_DeterministicOrder(t *testing.T) {
	data := map[string]string{
		"z-key": "z-value",
		"a-key": "a-value",
		"m-key": "m-value",
	}

	first := configStringDataHash(data)
	for i := range 100 {
		got := configStringDataHash(data)
		if got != first {
			t.Fatalf("configStringDataHash produced different results: %q vs %q (iteration %d)", first, got, i)
		}
	}
}

func TestConfigStringDataHash_NilMap_ReturnsEmpty(t *testing.T) {
	got := configStringDataHash(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil input, got %q", got)
	}
}

func TestConfigDataHash_Length(t *testing.T) {
	data := map[string][]byte{"key": []byte("value")}
	got := configDataHash(data)
	if len(got) != 32 {
		t.Fatalf("expected hash length 32, got %d (%q)", len(got), got)
	}
}

func TestMergeAnnotationsWithChecksum_PreservesUserAnnotations(t *testing.T) {
	user := map[string]string{
		"app.io/custom": "value",
		"another":       "annotation",
	}

	result := mergeAnnotationsWithChecksum(user, "checksum/config", "abc123")

	if result["app.io/custom"] != "value" {
		t.Fatalf("expected user annotation preserved, got %q", result["app.io/custom"])
	}
	if result["another"] != "annotation" {
		t.Fatalf("expected user annotation preserved, got %q", result["another"])
	}
	if result["checksum/config"] != "abc123" {
		t.Fatalf("expected checksum annotation, got %q", result["checksum/config"])
	}
}

func TestMergeAnnotationsWithChecksum_OverwritesUserChecksum(t *testing.T) {
	user := map[string]string{
		"checksum/config": "user-value",
	}

	result := mergeAnnotationsWithChecksum(user, "checksum/config", "controller-value")

	if result["checksum/config"] != "controller-value" {
		t.Fatalf("expected controller checksum to take precedence, got %q", result["checksum/config"])
	}
}

func TestMergeAnnotationsWithChecksum_NilAnnotations(t *testing.T) {
	result := mergeAnnotationsWithChecksum(nil, "checksum/config", "abc123")

	if result["checksum/config"] != "abc123" {
		t.Fatalf("expected checksum annotation, got %q", result["checksum/config"])
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 annotation, got %d", len(result))
	}
}

func TestMergeAnnotationsWithChecksum_EmptyChecksum(t *testing.T) {
	user := map[string]string{"app/custom": "value"}

	result := mergeAnnotationsWithChecksum(user, "checksum/config", "")

	if _, ok := result["checksum/config"]; ok {
		t.Fatal("expected no checksum annotation for empty value")
	}
	if result["app/custom"] != "value" {
		t.Fatalf("expected user annotation preserved, got %q", result["app/custom"])
	}
}
