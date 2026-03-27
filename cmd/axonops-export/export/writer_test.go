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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDryRun(t *testing.T) {
	resources := []Resource{
		{Kind: "AxonOpsMetricAlert", Name: "alert-1"},
		{Kind: "AxonOpsMetricAlert", Name: "alert-2"},
		{Kind: "Secret", Name: "my-secret"},
	}

	var buf bytes.Buffer
	if err := writeDryRun(resources, &buf); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "3 resources") {
		t.Errorf("expected '3 resources' in output, got: %s", out)
	}
	if !strings.Contains(out, "AxonOpsMetricAlert: 2") {
		t.Errorf("expected 'AxonOpsMetricAlert: 2' in output, got: %s", out)
	}
	if !strings.Contains(out, "Secret: 1") {
		t.Errorf("expected 'Secret: 1' in output, got: %s", out)
	}
}

func TestWriteDir(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "exported")

	resources := []Resource{
		{Kind: "Secret", Name: "my-secret", Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "my-secret"},
		}},
		{Kind: "AxonOpsMetricAlert", Name: "alert-1", Object: map[string]any{
			"apiVersion": "alerts.axonops.com/v1alpha1",
			"kind":       "AxonOpsMetricAlert",
			"metadata":   map[string]any{"name": "alert-1"},
		}},
	}

	if err := writeDir(resources, outDir); err != nil {
		t.Fatal(err)
	}

	// Verify files created
	files := []string{"secret-my-secret.yaml", "axonopsmetricalert-alert-1.yaml", "kustomization.yaml"}
	for _, f := range files {
		path := filepath.Join(outDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", path)
		}
	}

	// Verify kustomization.yaml references both files
	kdata, err := os.ReadFile(filepath.Join(outDir, "kustomization.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(kdata), "secret-my-secret.yaml") {
		t.Errorf("kustomization.yaml should reference secret-my-secret.yaml")
	}
	if !strings.Contains(string(kdata), "axonopsmetricalert-alert-1.yaml") {
		t.Errorf("kustomization.yaml should reference axonopsmetricalert-alert-1.yaml")
	}
}

func TestWriteStream(t *testing.T) {
	dir := t.TempDir()
	outFile := filepath.Join(dir, "out.yaml")

	resources := []Resource{
		{Kind: "Secret", Name: "s1", Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
		}},
		{Kind: "AxonOpsMetricAlert", Name: "a1", Object: map[string]any{
			"apiVersion": "alerts.axonops.com/v1alpha1",
			"kind":       "AxonOpsMetricAlert",
		}},
	}

	if err := writeStream(resources, outFile); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	if !strings.Contains(content, "---") {
		t.Error("multi-doc YAML should contain ---")
	}
	if !strings.Contains(content, "Secret") {
		t.Error("output should contain Secret")
	}
	if !strings.Contains(content, "AxonOpsMetricAlert") {
		t.Error("output should contain AxonOpsMetricAlert")
	}
}
