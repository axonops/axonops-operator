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
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/axonops/axonops-operator/internal/axonops"
)

// resolveHost determines the effective AxonOps host. When no custom host is
// provided it replicates the Terraform provider logic:
//   - probe {orgID}.axonops.cloud/dashboard/ — if it returns JSON the org
//     uses SAML and the host becomes {orgID}.axonops.cloud/dashboard
//   - otherwise the host becomes dash.axonops.cloud/{orgID}
//
// A custom host is used as-is (SAML /dashboard suffix is still appended when
// the probe returns JSON, matching provider behaviour for self-hosted SAML).
func resolveHost(protocol, host, orgID string, tlsSkipVerify bool) string {
	if host == "" {
		samlHost := orgID + ".axonops.cloud"
		if detectSAML(protocol, samlHost, tlsSkipVerify) {
			return samlHost + "/dashboard"
		}
		return "dash.axonops.cloud/" + orgID
	}
	if detectSAML(protocol, host, tlsSkipVerify) {
		return host + "/dashboard"
	}
	return host
}

// detectSAML probes {protocol}://{host}/dashboard/ and returns true when the
// response Content-Type is application/json (the SAML IDP redirect payload).
func detectSAML(protocol, host string, tlsSkipVerify bool) bool {
	probeURL := fmt.Sprintf("%s://%s/dashboard/", protocol, host)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: tlsSkipVerify}, //nolint:gosec
	}
	c := &http.Client{
		Timeout:   5 * time.Second,
		Transport: tr,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.Get(probeURL) //nolint:noctx
	if resp != nil {
		resp.Body.Close() //nolint:errcheck
	}
	return err == nil && resp != nil &&
		resp.StatusCode != http.StatusNotFound &&
		strings.Contains(resp.Header.Get("Content-Type"), "application/json")
}

// Run executes the export pipeline: connect, fetch, convert, write.
func Run(ctx context.Context, opts *Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	// Resolve effective host (handles SaaS org-path and SAML auto-detection).
	opts.Host = resolveHost(opts.Protocol, opts.Host, opts.OrgID, opts.TLSSkipVerify)

	// Resolve token type: explicit flag overrides auto-detection.
	tokenType := opts.TokenType
	if tokenType == "" {
		tokenType = axonops.DefaultTokenType(opts.Host)
	}
	opts.TokenType = tokenType

	// Create API client
	clientOpts := []axonops.ClientOption{}
	if opts.Verbose {
		clientOpts = append(clientOpts, axonops.WithVerbose())
	}

	client, err := axonops.NewClient(
		opts.Host, opts.Protocol,
		opts.OrgID, opts.APIKey, tokenType,
		opts.TLSSkipVerify,
		clientOpts...,
	)
	if err != nil {
		return fmt.Errorf("creating API client: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Connected to %s://%s (org: %s)\n", opts.Protocol, opts.Host, opts.OrgID)
	fmt.Fprintf(os.Stderr, "Exporting cluster %q (type: %s)\n", opts.ClusterName, opts.ClusterType)

	var resources []Resource

	// Always generate AxonOpsConnection + Secret first
	resources = append(resources, buildConnectionResources(opts)...)

	type exportJob struct {
		label string
		fn    func(context.Context, *axonops.Client, *Options) ([]Resource, error)
	}
	jobs := []exportJob{
		{"metric alerts", exportMetricAlerts},
		{"log alerts", exportLogAlerts},
		{"healthchecks", exportHealthchecks},
		{"integrations", exportIntegrations},
		{"adaptive repair", exportAdaptiveRepair},
		{"scheduled repairs", exportScheduledRepairs},
		{"backups", exportBackups},
		{"silence windows", exportSilenceWindows},
	}

	for _, job := range jobs {
		items, err := job.fn(ctx, client, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to export %s: %v\n", job.label, err)
			continue
		}
		resources = append(resources, items...)
		fmt.Fprintf(os.Stderr, "  found %d %s\n", len(items), job.label)
	}

	if len(resources) == 0 {
		fmt.Fprintln(os.Stderr, "No resources to export")
		return nil
	}

	return WriteResources(resources, opts)
}
