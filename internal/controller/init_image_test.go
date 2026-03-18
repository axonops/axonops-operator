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
	"strings"
	"testing"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

func TestResolveInitImage_DefaultIsPinned(t *testing.T) {
	server := &corev1alpha1.AxonOpsServer{}

	got := resolveInitImage(server)

	if got != defaultInitImage {
		t.Fatalf("expected default %q, got %q", defaultInitImage, got)
	}
	if strings.HasSuffix(got, ":latest") {
		t.Fatalf("default init image must not use :latest, got %q", got)
	}
	if !strings.Contains(got, ":") {
		t.Fatalf("default init image must include a tag, got %q", got)
	}
}

func TestResolveInitImage_CustomImage(t *testing.T) {
	server := &corev1alpha1.AxonOpsServer{
		Spec: corev1alpha1.AxonOpsServerSpec{
			InitImage: "myregistry.io/busybox:1.36.0",
		},
	}

	got := resolveInitImage(server)

	if got != "myregistry.io/busybox:1.36.0" {
		t.Fatalf("expected custom image, got %q", got)
	}
}

func TestResolveInitImage_EmptyFallsBackToDefault(t *testing.T) {
	server := &corev1alpha1.AxonOpsServer{
		Spec: corev1alpha1.AxonOpsServerSpec{
			InitImage: "",
		},
	}

	got := resolveInitImage(server)

	if got != defaultInitImage {
		t.Fatalf("expected default %q for empty spec, got %q", defaultInitImage, got)
	}
}

func TestDefaultInitImage_NotLatest(t *testing.T) {
	if strings.HasSuffix(defaultInitImage, ":latest") {
		t.Fatalf("defaultInitImage must not use :latest tag, got %q", defaultInitImage)
	}
}
