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

package export

import "fmt"

// Options holds all CLI flags for the export command.
type Options struct {
	// Connection
	Host          string
	Protocol      string
	OrgID         string
	APIKey        string
	TokenType     string
	TLSSkipVerify bool

	// Scope
	ClusterName string
	ClusterType string

	// Output
	Namespace      string
	ConnectionName string
	OutputDir      string
	Output         string
	DryRun         bool

	// Debug
	Verbose bool
}

// Validate checks that required fields are set.
func (o *Options) Validate() error {
	if o.OrgID == "" {
		return fmt.Errorf("--org-id is required")
	}
	if o.APIKey == "" {
		return fmt.Errorf("--api-key is required")
	}
	if o.ClusterName == "" {
		return fmt.Errorf("--cluster-name is required")
	}
	return nil
}
