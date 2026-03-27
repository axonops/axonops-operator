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
	"regexp"
	"strings"
)

var nonDNSChars = regexp.MustCompile(`[^a-z0-9-]`)
var multiDash = regexp.MustCompile(`-{2,}`)

// toDNSLabel converts an arbitrary string into a valid Kubernetes DNS label.
// The result is deterministic: the same input always produces the same output.
// DNS labels must be lowercase, alphanumeric or hyphens, start and end with
// an alphanumeric character, and be at most 63 characters.
func toDNSLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	// Replace common separators with hyphens
	s = strings.NewReplacer(" ", "-", "_", "-", ".", "-", "/", "-").Replace(s)
	// Remove anything that isn't alphanumeric or hyphen
	s = nonDNSChars.ReplaceAllString(s, "")
	// Collapse consecutive hyphens
	s = multiDash.ReplaceAllString(s, "-")
	// Trim leading/trailing hyphens
	s = strings.Trim(s, "-")
	// Truncate to 63 characters (DNS label max)
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "unnamed"
	}
	return s
}
