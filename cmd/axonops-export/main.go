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

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/axonops/axonops-operator/cmd/axonops-export/export"
)

func main() {
	root := &cobra.Command{
		Use:   "axonops-export",
		Short: "Export AxonOps configuration as Kubernetes CRD YAMLs",
		Long: `axonops-export connects to a running AxonOps instance and generates
Kubernetes YAML manifests for the axonops-operator CRDs. This enables
migration from Terraform, Ansible, or manual configuration to the
Kubernetes operator.`,
		SilenceUsage: true,
	}

	opts := &export.Options{}

	// Persistent flags (shared across subcommands)
	pf := root.PersistentFlags()
	pf.StringVar(&opts.Host, "host", "",
		"AxonOps host (default: auto-detected from org-id; SaaS uses dash.axonops.cloud/{org-id})")
	pf.StringVar(&opts.Protocol, "protocol", "https", "Protocol (http or https)")
	pf.StringVar(&opts.OrgID, "org-id", "", "Organization ID (required)")
	pf.StringVar(&opts.APIKey, "api-key", "", "API key (required)")
	pf.StringVar(&opts.TokenType, "token-type", "",
		"Auth token type (Bearer for cloud, AxonApi for self-hosted; auto-detected from host if omitted)")
	pf.StringVar(&opts.ClusterName, "cluster-name", "", "Cluster name to export from (required)")
	pf.StringVar(&opts.ClusterType, "cluster-type", "cassandra", "Cluster type (cassandra, kafka, dse)")
	pf.BoolVar(&opts.TLSSkipVerify, "tls-skip-verify", false, "Skip TLS certificate verification")
	pf.BoolVar(&opts.Verbose, "verbose", false, "Log HTTP requests and responses to stderr (auth header redacted)")
	pf.StringVar(&opts.Namespace, "namespace", "default", "Kubernetes namespace for generated resources")
	pf.StringVar(&opts.ConnectionName, "connection-name", "axonops-connection",
		"Name for the generated AxonOpsConnection resource")

	// Export command
	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export AxonOps configuration as CRD YAMLs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return export.Run(cmd.Context(), opts)
		},
	}
	exportCmd.Flags().StringVar(&opts.OutputDir, "output-dir", "", "Write one YAML per resource to this directory")
	exportCmd.Flags().StringVar(&opts.Output, "output", "-", "Output file (- for stdout)")
	exportCmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Print summary without writing files")

	root.AddCommand(exportCmd)

	// Make export the default command
	root.RunE = exportCmd.RunE
	// Copy export flags to root so they work without the subcommand
	root.Flags().AddFlagSet(exportCmd.Flags())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
