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

// exportSilenceWindows fetches silence windows and returns CRD resources.
func exportSilenceWindows(ctx context.Context, client *axonops.Client, opts *Options) ([]Resource, error) {
	silences, err := client.GetSilenceWindows(ctx, opts.ClusterType, opts.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("fetching silence windows: %w", err)
	}
	if len(silences) == 0 {
		return nil, nil
	}

	var resources []Resource
	for i, sw := range silences {
		name := toDNSLabel(fmt.Sprintf("%s-silence-%d", opts.ClusterName, i+1))
		if sw.CronExpr != "" {
			name = toDNSLabel(fmt.Sprintf("%s-silence-%s", opts.ClusterName, sw.CronExpr))
		}

		active := sw.Active
		obj := &alertsv1alpha1.AxonOpsSilenceWindow{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "alerts.axonops.com/v1alpha1",
				Kind:       "AxonOpsSilenceWindow",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: opts.Namespace,
			},
			Spec: alertsv1alpha1.AxonOpsSilenceWindowSpec{
				ConnectionRef:  opts.ConnectionName,
				ClusterName:    opts.ClusterName,
				ClusterType:    opts.ClusterType,
				Duration:       sw.Duration,
				Active:         &active,
				Recurring:      sw.IsRecurring,
				CronExpression: sw.CronExpr,
				Datacenters:    sw.DCs,
			},
		}
		resources = append(resources, Resource{Kind: "AxonOpsSilenceWindow", Name: name, Object: obj})
	}
	return resources, nil
}
