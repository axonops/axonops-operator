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

var _ = Describe("AxonOpsAdaptiveRepair Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-adaptive-repair"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		adaptiveRepair := &alertsv1alpha1.AxonOpsAdaptiveRepair{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsAdaptiveRepair")
			err := k8sClient.Get(ctx, typeNamespacedName, adaptiveRepair)
			if err != nil && errors.IsNotFound(err) {
				active := true
				resource := &alertsv1alpha1.AxonOpsAdaptiveRepair{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: alertsv1alpha1.AxonOpsAdaptiveRepairSpec{
						ConnectionRef: "test-connection",
						ClusterName:   "test-cluster",
						ClusterType:   "cassandra",
						Active:        &active,
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &alertsv1alpha1.AxonOpsAdaptiveRepair{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizer if present to allow cleanup
				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				By("Cleanup the specific resource instance AxonOpsAdaptiveRepair")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource and add finalizer", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AxonOpsAdaptiveRepairReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the finalizer was added
			resource := &alertsv1alpha1.AxonOpsAdaptiveRepair{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Finalizers).To(ContainElement(adaptiveRepairFinalizer))
		})

		It("should set Failed condition when AxonOpsConnection does not exist", func() {
			By("Reconciling the resource with a missing connection")
			controllerReconciler := &AxonOpsAdaptiveRepairReconciler{
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

			// Verify Failed condition
			resource := &alertsv1alpha1.AxonOpsAdaptiveRepair{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Conditions).NotTo(BeEmpty())
			failedCond := findCondition(resource.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(string(failedCond.Status)).To(Equal("True"))
			Expect(failedCond.Reason).To(Equal(ReasonConnectionError))
		})
	})
})
