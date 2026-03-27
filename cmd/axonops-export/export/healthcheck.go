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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

// exportHealthchecks fetches all healthcheck types and returns CRD resources.
func exportHealthchecks(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	hc, err := client.GetHealthchecks(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching healthchecks: %w", err)
	}

	var resources []Resource

	for _, h := range hc.HTTPChecks {
		r := httpHealthcheckToResource(h, opts)
		resources = append(resources, r)
	}
	for _, t := range hc.TCPChecks {
		r := tcpHealthcheckToResource(t, opts)
		resources = append(resources, r)
	}
	for _, s := range hc.ShellChecks {
		r := shellHealthcheckToResource(s, opts)
		resources = append(resources, r)
	}

	return resources, nil
}

func httpHealthcheckToResource(h axonops.HealthcheckHTTP, opts *Options) Resource {
	name := toDNSLabel(h.Name)

	// Parse headers from "Key: Value" strings to map
	headers := make(map[string]string)
	for _, hdr := range h.Headers {
		if i := indexByte(hdr, ':'); i >= 0 {
			headers[trimSpace(hdr[:i])] = trimSpace(hdr[i+1:])
		}
	}

	obj := &alertsv1alpha1.AxonOpsHealthcheckHTTP{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "alerts.axonops.com/v1alpha1",
			Kind:       "AxonOpsHealthcheckHTTP",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Spec: alertsv1alpha1.AxonOpsHealthcheckHTTPSpec{
			ConnectionRef:       opts.ConnectionName,
			ClusterName:         opts.ClusterName,
			ClusterType:         opts.ClusterType,
			Name:                h.Name,
			URL:                 h.HTTP,
			Method:              h.Method,
			Body:                h.Body,
			ExpectedStatus:      h.ExpectedStatus,
			Interval:            h.Interval,
			Timeout:             h.Timeout,
			Readonly:            h.Readonly,
			TLSSkipVerify:       h.TLSSkipVerify,
			SupportedAgentTypes: h.SupportedAgentTypes,
		},
	}
	if len(headers) > 0 {
		obj.Spec.Headers = headers
	}

	return Resource{Kind: "AxonOpsHealthcheckHTTP", Name: name, Object: obj}
}

func tcpHealthcheckToResource(t axonops.HealthcheckTCP, opts *Options) Resource {
	name := toDNSLabel(t.Name)

	return Resource{
		Kind: "AxonOpsHealthcheckTCP",
		Name: name,
		Object: &alertsv1alpha1.AxonOpsHealthcheckTCP{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "alerts.axonops.com/v1alpha1",
				Kind:       "AxonOpsHealthcheckTCP",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: opts.Namespace,
			},
			Spec: alertsv1alpha1.AxonOpsHealthcheckTCPSpec{
				ConnectionRef:       opts.ConnectionName,
				ClusterName:         opts.ClusterName,
				ClusterType:         opts.ClusterType,
				Name:                t.Name,
				TCP:                 t.TCP,
				Interval:            t.Interval,
				Timeout:             t.Timeout,
				Readonly:            t.Readonly,
				SupportedAgentTypes: t.SupportedAgentTypes,
			},
		},
	}
}

func shellHealthcheckToResource(s axonops.HealthcheckShell, opts *Options) Resource {
	name := toDNSLabel(s.Name)

	return Resource{
		Kind: "AxonOpsHealthcheckShell",
		Name: name,
		Object: &alertsv1alpha1.AxonOpsHealthcheckShell{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "alerts.axonops.com/v1alpha1",
				Kind:       "AxonOpsHealthcheckShell",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: opts.Namespace,
			},
			Spec: alertsv1alpha1.AxonOpsHealthcheckShellSpec{
				ConnectionRef: opts.ConnectionName,
				ClusterName:   opts.ClusterName,
				ClusterType:   opts.ClusterType,
				Name:          s.Name,
				Script:        s.Script,
				Shell:         s.Shell,
				Interval:      s.Interval,
				Timeout:       s.Timeout,
				Readonly:      s.Readonly,
			},
		},
	}
}

// indexByte returns the index of b in s, or -1.
func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// trimSpace trims ASCII whitespace from both ends.
func trimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
