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

package alerts

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
)

var _ = Describe("AxonOpsMetricAlert Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		axonopsmetricalert := &alertsv1alpha1.AxonOpsMetricAlert{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsMetricAlert")
			err := k8sClient.Get(ctx, typeNamespacedName, axonopsmetricalert)
			if err != nil && errors.IsNotFound(err) {
				resource := &alertsv1alpha1.AxonOpsMetricAlert{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: alertsv1alpha1.AxonOpsMetricAlertSpec{
						ClusterName:   "test-cluster",
						ClusterType:   "cassandra",
						Name:          "test-alert",
						Operator:      ">",
						WarningValue:  50,
						CriticalValue: 100,
						Duration:      "15m",
						Dashboard:     "Test Dashboard",
						Chart:         "Test Chart",
						Metric:        "test_metric",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &alertsv1alpha1.AxonOpsMetricAlert{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AxonOpsMetricAlert")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AxonOpsMetricAlertReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
