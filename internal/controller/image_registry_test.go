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

package controller

import (
	"testing"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

const (
	testImageGHCR       = "ghcr.io/axonops/img"
	testImageLocal      = "registry.local/axonops/img"
	testRegistryLocal   = "registry.local"
	testRegistryHarbor  = "harbor.internal.com"
	testInitImageLocal  = "registry.local/library/busybox:1.37.0"
	testCustomInitImage = "my.io/custom-busybox:2.0"
)

func TestReplaceImageRegistry(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		newRegistry string
		want        string
	}{
		{
			name:        "ExplicitRegistry_SingleSegmentPath",
			image:       testImageGHCR,
			newRegistry: testRegistryLocal,
			want:        testImageLocal,
		},
		{
			name:        "ExplicitRegistry_MultiSegmentPath",
			image:       "registry.axonops.com/axonops-public/axonops-docker/axon-server",
			newRegistry: testRegistryLocal,
			want:        "registry.local/axonops-public/axonops-docker/axon-server",
		},
		{
			name:        "ExplicitRegistry_WithPort",
			image:       testImageGHCR,
			newRegistry: "registry.local:5000",
			want:        "registry.local:5000/axonops/img",
		},
		{
			name:        "DockerHubImage_FullyQualified",
			image:       defaultInitImage,
			newRegistry: testRegistryLocal,
			want:        testInitImageLocal,
		},
		{
			name:        "DockerHubImage_ShortForm",
			image:       "busybox:1.37.0",
			newRegistry: testRegistryLocal,
			want:        "registry.local/busybox:1.37.0",
		},
		{
			name:        "DockerHubImage_LibraryPath",
			image:       "library/nginx:latest",
			newRegistry: testRegistryLocal,
			want:        "registry.local/library/nginx:latest",
		},
		{
			name:        "EmptyRegistry_ReturnsOriginal",
			image:       testImageGHCR,
			newRegistry: "",
			want:        testImageGHCR,
		},
		{
			name:        "TrailingSlash_Stripped",
			image:       testImageGHCR,
			newRegistry: "registry.local/",
			want:        testImageLocal,
		},
		{
			name:        "RegistryWithPort_ColonInFirstSegment",
			image:       "localhost:5000/myimg",
			newRegistry: testRegistryLocal,
			want:        "registry.local/myimg",
		},
		{
			name:        "ImageOnly_NoTag",
			image:       testImageBusybox,
			newRegistry: testRegistryLocal,
			want:        "registry.local/busybox",
		},
		{
			name:        "AllDefaultImages_TimeSeries",
			image:       defaultTimeseriesImage,
			newRegistry: testRegistryHarbor,
			want:        "harbor.internal.com/axonops/axondb-timeseries",
		},
		{
			name:        "AllDefaultImages_Search",
			image:       defaultSearchImage,
			newRegistry: testRegistryHarbor,
			want:        "harbor.internal.com/axonops/axondb-search",
		},
		{
			name:        "AllDefaultImages_Server",
			image:       defaultServerImage,
			newRegistry: testRegistryHarbor,
			want:        "harbor.internal.com/axonops-public/axonops-docker/axon-server",
		},
		{
			name:        "AllDefaultImages_Dashboard",
			image:       defaultDashboardImage,
			newRegistry: testRegistryHarbor,
			want:        "harbor.internal.com/axonops-public/axonops-docker/axon-dash",
		},
		{
			name:        "AllDefaultImages_InitContainer",
			image:       defaultInitImage,
			newRegistry: testRegistryHarbor,
			want:        "harbor.internal.com/library/busybox:1.37.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := replaceImageRegistry(tt.image, tt.newRegistry)
			if got != tt.want {
				t.Errorf("replaceImageRegistry(%q, %q) = %q, want %q", tt.image, tt.newRegistry, got, tt.want)
			}
		})
	}
}

func TestResolveImage(t *testing.T) {
	tests := []struct {
		name              string
		defaultImage      string
		globalRegistry    string
		componentOverride string
		want              string
	}{
		{
			name:              "ComponentOverride_TakesPrecedence",
			defaultImage:      testImageGHCR,
			globalRegistry:    testRegistryLocal,
			componentOverride: "custom.io/my-img",
			want:              "custom.io/my-img",
		},
		{
			name:              "GlobalRegistry_Applied",
			defaultImage:      testImageGHCR,
			globalRegistry:    testRegistryLocal,
			componentOverride: "",
			want:              testImageLocal,
		},
		{
			name:              "Default_WhenNothingSet",
			defaultImage:      testImageGHCR,
			globalRegistry:    "",
			componentOverride: "",
			want:              testImageGHCR,
		},
		{
			name:              "ComponentOverride_IgnoresGlobalRegistry",
			defaultImage:      defaultInitImage,
			globalRegistry:    testRegistryLocal,
			componentOverride: testCustomInitImage,
			want:              testCustomInitImage,
		},
		{
			name:              "GlobalRegistry_DockerHubImage",
			defaultImage:      defaultInitImage,
			globalRegistry:    testRegistryLocal,
			componentOverride: "",
			want:              testInitImageLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveImage(tt.defaultImage, tt.globalRegistry, tt.componentOverride)
			if got != tt.want {
				t.Errorf("resolveImage(%q, %q, %q) = %q, want %q",
					tt.defaultImage, tt.globalRegistry, tt.componentOverride, got, tt.want)
			}
		})
	}
}

func TestResolveInitImage_ImageRegistry(t *testing.T) {
	tests := []struct {
		name   string
		server *corev1alpha1.AxonOpsPlatform
		want   string
	}{
		{
			name:   "Default",
			server: &corev1alpha1.AxonOpsPlatform{},
			want:   defaultInitImage,
		},
		{
			name: "CustomInitImage_TakesPrecedence",
			server: &corev1alpha1.AxonOpsPlatform{
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					ImageRegistry: testRegistryLocal,
					InitImage:     testCustomInitImage,
				},
			},
			want: testCustomInitImage,
		},
		{
			name: "ImageRegistry_AppliedToDefault",
			server: &corev1alpha1.AxonOpsPlatform{
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					ImageRegistry: testRegistryLocal,
				},
			},
			want: testInitImageLocal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveInitImage(tt.server)
			if got != tt.want {
				t.Errorf("resolveInitImage() = %q, want %q", got, tt.want)
			}
		})
	}
}
