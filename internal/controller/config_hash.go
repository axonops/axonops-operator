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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"sort"
)

// configDataHash computes a deterministic SHA-256 hash of config data.
// Keys are sorted lexicographically before hashing to ensure identical
// output regardless of Go map iteration order. Returns an empty string
// for nil or empty input.
func configDataHash(data map[string][]byte) string {
	if len(data) == 0 {
		return ""
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\n", k, data[k])
	}

	return hex.EncodeToString(h.Sum(nil))[:32]
}

// configStringDataHash computes a deterministic SHA-256 hash of string config data.
// Used for ConfigMap .Data fields which are map[string]string.
func configStringDataHash(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\n", k, data[k])
	}

	return hex.EncodeToString(h.Sum(nil))[:32]
}

// mergeAnnotationsWithChecksum merges a checksum annotation into user-provided annotations.
// The checksum annotation always takes precedence over user-provided values.
func mergeAnnotationsWithChecksum(userAnnotations map[string]string, checksumKey, checksumValue string) map[string]string {
	result := make(map[string]string, len(userAnnotations)+1)
	maps.Copy(result, userAnnotations)
	if checksumValue != "" {
		result[checksumKey] = checksumValue
	}
	return result
}
