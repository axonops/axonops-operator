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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// Resource is a single Kubernetes resource to be written.
type Resource struct {
	Kind string
	Name string
	// Object is the Go struct to marshal (must be a Kubernetes object or map).
	Object any
}

// WriteResources writes resources according to output options.
// If opts.DryRun is true, it prints a summary instead.
// If opts.OutputDir is set, it writes one file per resource + kustomization.yaml.
// Otherwise it writes multi-document YAML to opts.Output (or stdout for "-").
func WriteResources(resources []Resource, opts *Options) error {
	if opts.DryRun {
		return writeDryRun(resources, os.Stdout)
	}
	if opts.OutputDir != "" {
		return writeDir(resources, opts.OutputDir)
	}
	return writeStream(resources, opts.Output)
}

func writeDryRun(resources []Resource, w io.Writer) error {
	counts := map[string]int{}
	for _, r := range resources {
		counts[r.Kind]++
	}
	if _, err := fmt.Fprintf(w, "Dry run — %d resources would be exported:\n", len(resources)); err != nil {
		return err
	}
	for kind, n := range counts {
		if _, err := fmt.Fprintf(w, "  %s: %d\n", kind, n); err != nil {
			return err
		}
	}
	return nil
}

func writeDir(resources []Resource, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	var filenames []string
	for _, r := range resources {
		data, err := marshalYAML(r.Object)
		if err != nil {
			return fmt.Errorf("marshalling %s/%s: %w", r.Kind, r.Name, err)
		}
		fname := fmt.Sprintf("%s-%s.yaml", strings.ToLower(r.Kind), r.Name)
		fpath := filepath.Join(dir, fname)
		if err := os.WriteFile(fpath, data, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", fpath, err)
		}
		filenames = append(filenames, fname)
		fmt.Fprintf(os.Stderr, "  wrote %s\n", fpath)
	}

	// Write kustomization.yaml
	var buf bytes.Buffer
	buf.WriteString("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n")
	for _, f := range filenames {
		fmt.Fprintf(&buf, "  - %s\n", f)
	}
	kpath := filepath.Join(dir, "kustomization.yaml")
	if err := os.WriteFile(kpath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing kustomization.yaml: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  wrote %s\n", kpath)
	return nil
}

func writeStream(resources []Resource, output string) (retErr error) {
	var w io.Writer
	if output == "-" || output == "" {
		w = os.Stdout
	} else {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer func() {
			if err := f.Close(); err != nil && retErr == nil {
				retErr = fmt.Errorf("closing output file: %w", err)
			}
		}()
		w = f
	}

	for i, r := range resources {
		if i > 0 {
			if _, err := fmt.Fprintln(w, "---"); err != nil {
				return fmt.Errorf("writing separator: %w", err)
			}
		}
		data, err := marshalYAML(r.Object)
		if err != nil {
			return fmt.Errorf("marshalling %s/%s: %w", r.Kind, r.Name, err)
		}
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf("writing %s/%s: %w", r.Kind, r.Name, err)
		}
	}
	return nil
}

func marshalYAML(obj any) ([]byte, error) {
	data, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return data, nil
}
