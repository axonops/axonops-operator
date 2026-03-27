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

import (
	"context"
	"fmt"
	"os"

	"github.com/axonops/axonops-operator/internal/axonops"
)

// Run executes the export pipeline: connect, fetch, convert, write.
func Run(ctx context.Context, opts *Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	// Resolve token type: explicit flag overrides auto-detection.
	tokenType := opts.TokenType
	if tokenType == "" {
		tokenType = axonops.DefaultTokenType(opts.Host)
	}
	opts.TokenType = tokenType

	// Create API client
	client, err := axonops.NewClient(
		opts.Host, opts.Protocol,
		opts.OrgID, opts.APIKey, tokenType,
		opts.TLSSkipVerify,
	)
	if err != nil {
		return fmt.Errorf("creating API client: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Connected to %s://%s (org: %s)\n", opts.Protocol, opts.Host, opts.OrgID)
	fmt.Fprintf(os.Stderr, "Exporting cluster %q (type: %s)\n", opts.ClusterName, opts.ClusterType)

	var resources []Resource

	// Always generate AxonOpsConnection + Secret first
	resources = append(resources, buildConnectionResources(opts)...)

	// Export metric alerts
	metricAlerts, err := exportMetricAlerts(ctx, client, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to export metric alerts: %v\n", err)
	} else {
		resources = append(resources, metricAlerts...)
		fmt.Fprintf(os.Stderr, "  found %d metric alert(s)\n", len(metricAlerts))
	}

	// Future: export log alerts, healthchecks, alert routes, etc.

	if len(resources) == 0 {
		fmt.Fprintln(os.Stderr, "No resources to export")
		return nil
	}

	return WriteResources(resources, opts)
}
