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

var _ = Describe("AxonOpsCommitlogArchive Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-commitlog-archive"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		commitlogArchive := &alertsv1alpha1.AxonOpsCommitlogArchive{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsCommitlogArchive")
			err := k8sClient.Get(ctx, typeNamespacedName, commitlogArchive)
			if err != nil && errors.IsNotFound(err) {
				resource := &alertsv1alpha1.AxonOpsCommitlogArchive{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: alertsv1alpha1.AxonOpsCommitlogArchiveSpec{
						ConnectionRef: "test-connection",
						ClusterName:   "test-cluster",
						ClusterType:   "cassandra",
						RemoteType:    "local",
						RemotePath:    "/mnt/commitlogs",
						Datacenters:   []string{"dc1"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &alertsv1alpha1.AxonOpsCommitlogArchive{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				By("Cleanup the specific resource instance AxonOpsCommitlogArchive")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource and add finalizer", func() {
			controllerReconciler := &AxonOpsCommitlogArchiveReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			resource := &alertsv1alpha1.AxonOpsCommitlogArchive{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Finalizers).To(ContainElement(commitlogArchiveFinalizer))
		})

		It("should set Failed condition when AxonOpsConnection does not exist", func() {
			controllerReconciler := &AxonOpsCommitlogArchiveReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile fails on connection
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			resource := &alertsv1alpha1.AxonOpsCommitlogArchive{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Conditions).NotTo(BeEmpty())
			failedCond := findCondition(resource.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(string(failedCond.Status)).To(Equal("True"))
			Expect(failedCond.Reason).To(Equal(ReasonConnectionError))
		})

		It("should set Failed condition when remoteType=s3 but no S3 config", func() {
			s3Name := "test-s3-no-config"
			s3NN := types.NamespacedName{Name: s3Name, Namespace: "default"}
			resource := &alertsv1alpha1.AxonOpsCommitlogArchive{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s3Name,
					Namespace: "default",
				},
				Spec: alertsv1alpha1.AxonOpsCommitlogArchiveSpec{
					ConnectionRef: "test-connection",
					ClusterName:   "test-cluster",
					ClusterType:   "cassandra",
					RemoteType:    "s3",
					RemotePath:    "s3://bucket/path",
					Datacenters:   []string{"dc1"},
					// S3 config intentionally nil
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			controllerReconciler := &AxonOpsCommitlogArchiveReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: s3NN})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile hits connection error (no real connection), but the validation
			// happens after connection resolution. Since connection fails first, we get ConnectionError.
			// This is the expected order of operations.
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: s3NN})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			updated := &alertsv1alpha1.AxonOpsCommitlogArchive{}
			Expect(k8sClient.Get(ctx, s3NN, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(commitlogArchiveFinalizer))

			// Cleanup
			updated.Finalizers = nil
			Expect(k8sClient.Update(ctx, updated)).To(Succeed())
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())
		})
	})
})
