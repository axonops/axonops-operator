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

import "strings"

// replaceImageRegistry replaces the registry host portion of a container image
// reference with newRegistry. If newRegistry is empty, the image is returned
// unchanged.
//
// For images with an explicit registry (first path segment contains "." or ":"),
// that segment is replaced:
//
//	replaceImageRegistry("ghcr.io/axonops/img", "harbor.io") => "harbor.io/axonops/img"
//	replaceImageRegistry("registry.axonops.com/path/img", "harbor.io") => "harbor.io/path/img"
//
// For images without an explicit registry (e.g., Docker Hub library images),
// the registry is prepended:
//
//	replaceImageRegistry("busybox:1.37.0", "harbor.io") => "harbor.io/busybox:1.37.0"
func replaceImageRegistry(image, newRegistry string) string {
	if newRegistry == "" {
		return image
	}

	newRegistry = strings.TrimRight(newRegistry, "/")

	// Split image into segments by "/"
	parts := strings.SplitN(image, "/", 2)

	// Single segment (e.g., "busybox:1.37.0") — no explicit registry, prepend
	if len(parts) == 1 {
		return newRegistry + "/" + image
	}

	// Check if the first segment looks like a registry host (contains "." or ":")
	firstSegment := parts[0]
	if strings.ContainsAny(firstSegment, ".:") {
		// Replace the registry portion, keep the path
		return newRegistry + "/" + parts[1]
	}

	// No explicit registry (e.g., "library/nginx:latest") — prepend
	return newRegistry + "/" + image
}

// resolveImage determines the final container image repository to use.
// Precedence (highest to lowest):
//  1. componentOverride — per-component repository.image, used as-is
//  2. globalRegistry — replaces the registry host in defaultImage
//  3. defaultImage — the hardcoded default
func resolveImage(defaultImage, globalRegistry, componentOverride string) string {
	if componentOverride != "" {
		return componentOverride
	}
	if globalRegistry != "" {
		return replaceImageRegistry(defaultImage, globalRegistry)
	}
	return defaultImage
}
