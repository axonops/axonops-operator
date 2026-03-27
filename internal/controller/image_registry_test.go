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

func TestReplaceImageRegistry(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		newRegistry string
		want        string
	}{
		{
			name:        "ExplicitRegistry_SingleSegmentPath",
			image:       "ghcr.io/axonops/img",
			newRegistry: "registry.local",
			want:        "registry.local/axonops/img",
		},
		{
			name:        "ExplicitRegistry_MultiSegmentPath",
			image:       "registry.axonops.com/axonops-public/axonops-docker/axon-server",
			newRegistry: "registry.local",
			want:        "registry.local/axonops-public/axonops-docker/axon-server",
		},
		{
			name:        "ExplicitRegistry_WithPort",
			image:       "ghcr.io/axonops/img",
			newRegistry: "registry.local:5000",
			want:        "registry.local:5000/axonops/img",
		},
		{
			name:        "DockerHubImage_FullyQualified",
			image:       "docker.io/library/busybox:1.37.0",
			newRegistry: "registry.local",
			want:        "registry.local/library/busybox:1.37.0",
		},
		{
			name:        "DockerHubImage_ShortForm",
			image:       "busybox:1.37.0",
			newRegistry: "registry.local",
			want:        "registry.local/busybox:1.37.0",
		},
		{
			name:        "DockerHubImage_LibraryPath",
			image:       "library/nginx:latest",
			newRegistry: "registry.local",
			want:        "registry.local/library/nginx:latest",
		},
		{
			name:        "EmptyRegistry_ReturnsOriginal",
			image:       "ghcr.io/axonops/img",
			newRegistry: "",
			want:        "ghcr.io/axonops/img",
		},
		{
			name:        "TrailingSlash_Stripped",
			image:       "ghcr.io/axonops/img",
			newRegistry: "registry.local/",
			want:        "registry.local/axonops/img",
		},
		{
			name:        "RegistryWithPort_ColonInFirstSegment",
			image:       "localhost:5000/myimg",
			newRegistry: "registry.local",
			want:        "registry.local/myimg",
		},
		{
			name:        "ImageOnly_NoTag",
			image:       "busybox",
			newRegistry: "registry.local",
			want:        "registry.local/busybox",
		},
		{
			name:        "AllDefaultImages_TimeSeries",
			image:       defaultTimeseriesImage,
			newRegistry: "harbor.internal.com",
			want:        "harbor.internal.com/axonops/axondb-timeseries",
		},
		{
			name:        "AllDefaultImages_Search",
			image:       defaultSearchImage,
			newRegistry: "harbor.internal.com",
			want:        "harbor.internal.com/axonops/axondb-search",
		},
		{
			name:        "AllDefaultImages_Server",
			image:       defaultServerImage,
			newRegistry: "harbor.internal.com",
			want:        "harbor.internal.com/axonops-public/axonops-docker/axon-server",
		},
		{
			name:        "AllDefaultImages_Dashboard",
			image:       defaultDashboardImage,
			newRegistry: "harbor.internal.com",
			want:        "harbor.internal.com/axonops-public/axonops-docker/axon-dash",
		},
		{
			name:        "AllDefaultImages_InitContainer",
			image:       defaultInitImage,
			newRegistry: "harbor.internal.com",
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
			defaultImage:      "ghcr.io/axonops/img",
			globalRegistry:    "registry.local",
			componentOverride: "custom.io/my-img",
			want:              "custom.io/my-img",
		},
		{
			name:              "GlobalRegistry_Applied",
			defaultImage:      "ghcr.io/axonops/img",
			globalRegistry:    "registry.local",
			componentOverride: "",
			want:              "registry.local/axonops/img",
		},
		{
			name:              "Default_WhenNothingSet",
			defaultImage:      "ghcr.io/axonops/img",
			globalRegistry:    "",
			componentOverride: "",
			want:              "ghcr.io/axonops/img",
		},
		{
			name:              "ComponentOverride_IgnoresGlobalRegistry",
			defaultImage:      "docker.io/library/busybox:1.37.0",
			globalRegistry:    "registry.local",
			componentOverride: "my.io/custom-busybox:2.0",
			want:              "my.io/custom-busybox:2.0",
		},
		{
			name:              "GlobalRegistry_DockerHubImage",
			defaultImage:      "docker.io/library/busybox:1.37.0",
			globalRegistry:    "registry.local",
			componentOverride: "",
			want:              "registry.local/library/busybox:1.37.0",
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
		server *corev1alpha1.AxonOpsServer
		want   string
	}{
		{
			name:   "Default",
			server: &corev1alpha1.AxonOpsServer{},
			want:   defaultInitImage,
		},
		{
			name: "CustomInitImage_TakesPrecedence",
			server: &corev1alpha1.AxonOpsServer{
				Spec: corev1alpha1.AxonOpsServerSpec{
					ImageRegistry: "registry.local",
					InitImage:     "my.io/custom-busybox:2.0",
				},
			},
			want: "my.io/custom-busybox:2.0",
		},
		{
			name: "ImageRegistry_AppliedToDefault",
			server: &corev1alpha1.AxonOpsServer{
				Spec: corev1alpha1.AxonOpsServerSpec{
					ImageRegistry: "registry.local",
				},
			},
			want: "registry.local/library/busybox:1.37.0",
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
