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

var _ = Describe("AxonOpsAlertEndpoint Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-endpoint"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		axonopsalertendpoint := &alertsv1alpha1.AxonOpsAlertEndpoint{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsAlertEndpoint")
			err := k8sClient.Get(ctx, typeNamespacedName, axonopsalertendpoint)
			if err != nil && errors.IsNotFound(err) {
				resource := &alertsv1alpha1.AxonOpsAlertEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: alertsv1alpha1.AxonOpsAlertEndpointSpec{
						ConnectionRef: "test-connection",
						ClusterName:   "test-cluster",
						ClusterType:   "cassandra",
						Name:          "test-slack-endpoint",
						Type:          "slack",
						Slack: &alertsv1alpha1.SlackEndpointConfig{
							URL:     "https://hooks.slack.com/services/test",
							Channel: "#test-alerts",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &alertsv1alpha1.AxonOpsAlertEndpoint{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizer if present to allow cleanup
				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				By("Cleanup the specific resource instance AxonOpsAlertEndpoint")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource and add finalizer", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AxonOpsAlertEndpointReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the finalizer was added
			resource := &alertsv1alpha1.AxonOpsAlertEndpoint{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Finalizers).To(ContainElement(alertEndpointFinalizerName))
		})

		It("should set Ready=False when AxonOpsConnection does not exist", func() {
			By("Reconciling the resource with a missing connection")
			controllerReconciler := &AxonOpsAlertEndpointReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile tries to resolve connection and fails
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Verify Ready=False condition
			resource := &alertsv1alpha1.AxonOpsAlertEndpoint{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Conditions).NotTo(BeEmpty())
			readyCond := findCondition(resource.Status.Conditions, condTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(string(readyCond.Status)).To(Equal("False"))
			Expect(readyCond.Reason).To(Equal(ReasonConnectionError))
		})

		It("should set Ready=False with InvalidConfig when type-config mismatch", func() {
			By("Creating a slack endpoint without slack config")
			mismatchName := "test-mismatch"
			mismatchNN := types.NamespacedName{Name: mismatchName, Namespace: "default"}
			resource := &alertsv1alpha1.AxonOpsAlertEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mismatchName,
					Namespace: "default",
				},
				Spec: alertsv1alpha1.AxonOpsAlertEndpointSpec{
					ConnectionRef: "test-connection",
					ClusterName:   "test-cluster",
					ClusterType:   "cassandra",
					Name:          "test-endpoint",
					Type:          "pagerduty",
					// PagerDuty config intentionally nil
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &AxonOpsAlertEndpointReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: mismatchNN})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile detects mismatch
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: mismatchNN})
			Expect(err).NotTo(HaveOccurred())

			updated := &alertsv1alpha1.AxonOpsAlertEndpoint{}
			Expect(k8sClient.Get(ctx, mismatchNN, updated)).To(Succeed())
			readyCond := findCondition(updated.Status.Conditions, condTypeReady)
			Expect(readyCond).NotTo(BeNil())
			Expect(string(readyCond.Status)).To(Equal("False"))
			Expect(readyCond.Reason).To(Equal("InvalidConfig"))

			// Cleanup
			updated.Finalizers = nil
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())
		})
	})
})

func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}
