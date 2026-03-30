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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

// mockRESTMapper wraps the real RESTMapper but mocks cert-manager availability
// to enable tests to run without cert-manager CRDs installed.
type mockRESTMapper struct {
	delegate meta.RESTMapper
}

func (m *mockRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	// Mock cert-manager as available
	if gk.Group == "cert-manager.io" && gk.Kind == "Certificate" {
		return &meta.RESTMapping{
			Resource: schema.GroupVersionResource{
				Group:    "cert-manager.io",
				Version:  "v1",
				Resource: "certificates",
			},
		}, nil
	}
	return m.delegate.RESTMapping(gk, versions...)
}

func (m *mockRESTMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*meta.RESTMapping, error) {
	return m.delegate.RESTMappings(gk, versions...)
}

func (m *mockRESTMapper) ResourceSingularizer(resource string) (string, error) {
	return m.delegate.ResourceSingularizer(resource)
}

func (m *mockRESTMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	return m.delegate.ResourceFor(input)
}

func (m *mockRESTMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	return m.delegate.ResourcesFor(input)
}

func (m *mockRESTMapper) KindsFor(input schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	return m.delegate.KindsFor(input)
}

func (m *mockRESTMapper) KindFor(input schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	return m.delegate.KindFor(input)
}

var _ = Describe("AxonOpsPlatform Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		axonopsplatform := &corev1alpha1.AxonOpsPlatform{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsPlatform")
			err := k8sClient.Get(ctx, typeNamespacedName, axonopsplatform)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.AxonOpsPlatform{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &corev1alpha1.AxonOpsPlatform{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AxonOpsPlatform")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("External Search support", func() {
		It("should not create internal Search StatefulSet when external hosts are configured", func() {
			const resourceName = "external-search-test"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create a search auth secret for external search
			searchSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "external-search-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"AXONOPS_SEARCH_USER":     []byte("admin"),
					"AXONOPS_SEARCH_PASSWORD": []byte("password123"),
				},
			}
			Expect(k8sClient.Create(ctx, searchSecret)).To(Succeed())

			// Create AxonOpsPlatform with external search configured
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://opensearch.example.com:9200"},
								TLS: corev1alpha1.AxonTLSConfig{
									Enabled:            false,
									InsecureSkipVerify: false,
								},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "external-search-secret",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the external search resource")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that no internal Search StatefulSet was created")
			searchSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-search",
				Namespace: "default",
			}, searchSts)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Verifying that the status condition reflects Search: External")
			updatedResource := &corev1alpha1.AxonOpsPlatform{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedResource)).To(Succeed())
			cond := meta.FindStatusCondition(updatedResource.Status.Conditions, "SearchMode")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("External"))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, searchSecret)).To(Succeed())
		})

		It("should auto-generate credentials when external Search has no auth configured", func() {
			const resourceName = "external-search-no-auth"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create AxonOpsPlatform with external search but no credentials
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://opensearch.example.com:9200"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								// No SecretRef or Username - credentials will be auto-generated
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource with missing Search credentials")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that a managed auth secret was created with auto-generated credentials")
			managedSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-search-auth",
				Namespace: "default",
			}, managedSecret)).To(Succeed())
			Expect(managedSecret.Data).To(HaveKey("AXONOPS_SEARCH_USER"))
			Expect(managedSecret.Data).To(HaveKey("AXONOPS_SEARCH_PASSWORD"))
			Expect(string(managedSecret.Data["AXONOPS_SEARCH_USER"])).NotTo(BeEmpty())
			Expect(string(managedSecret.Data["AXONOPS_SEARCH_PASSWORD"])).NotTo(BeEmpty())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, managedSecret)).To(Succeed())
		})

		It("should exclude search-tls volume when Search is external", func() {
			const resourceName = "external-search-no-tls"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create a search auth secret
			searchSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "search-cred-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"AXONOPS_SEARCH_USER":     []byte("admin"),
					"AXONOPS_SEARCH_PASSWORD": []byte("password123"),
				},
			}
			Expect(k8sClient.Create(ctx, searchSecret)).To(Succeed())

			// Create AxonOpsPlatform with external search and minimal server config
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://opensearch.example.com:9200"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "search-cred-secret",
							},
						},
					},
					Server: &corev1alpha1.AxonServerComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
						},
						OrgName: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that Server StatefulSet has no search-tls volume")
			serverSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-server",
				Namespace: "default",
			}, serverSts)
			Expect(err).NotTo(HaveOccurred())

			// Check that search-tls volume is not present
			for _, vol := range serverSts.Spec.Template.Spec.Volumes {
				Expect(vol.Name).NotTo(Equal("search-tls"))
			}

			// Check that search-tls volume mount is not present
			for _, mount := range serverSts.Spec.Template.Spec.Containers[0].VolumeMounts {
				Expect(mount.Name).NotTo(Equal("search-tls"))
			}

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, searchSecret)).To(Succeed())
		})
	})

	Context("External TimeSeries support", func() {
		It("should not create internal TimeSeries StatefulSet when external hosts are configured", func() {
			const resourceName = "external-timeseries-test"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create a timeseries auth secret for external timeseries
			timeseriesSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "external-timeseries-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"AXONOPS_DB_USER":     []byte("axonops"),
					"AXONOPS_DB_PASSWORD": []byte("cassandra123"),
				},
			}
			Expect(k8sClient.Create(ctx, timeseriesSecret)).To(Succeed())

			// Create AxonOpsPlatform with external timeseries configured
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"cassandra-node1.example.com:9042", "cassandra-node2.example.com:9042"},
								TLS: corev1alpha1.AxonTLSConfig{
									Enabled:            false,
									InsecureSkipVerify: false,
								},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "external-timeseries-secret",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the external timeseries resource")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that no internal TimeSeries StatefulSet was created")
			timeseriesSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-timeseries",
				Namespace: "default",
			}, timeseriesSts)
			Expect(errors.IsNotFound(err)).To(BeTrue())

			By("Verifying that the status condition reflects TimeSeries: External")
			updatedResource := &corev1alpha1.AxonOpsPlatform{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedResource)).To(Succeed())
			cond := meta.FindStatusCondition(updatedResource.Status.Conditions, "TimeSeriesMode")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("External"))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, timeseriesSecret)).To(Succeed())
		})

		It("should auto-generate credentials when external TimeSeries has no auth configured", func() {
			const resourceName = "external-timeseries-no-auth"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create AxonOpsPlatform with external timeseries but no credentials
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"cassandra-node1.example.com:9042"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								// No SecretRef or Username - credentials will be auto-generated
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource with missing TimeSeries credentials")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that a managed auth secret was created with auto-generated credentials")
			managedSecret := &corev1.Secret{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-timeseries-auth",
				Namespace: "default",
			}, managedSecret)).To(Succeed())
			Expect(managedSecret.Data).To(HaveKey("AXONOPS_DB_USER"))
			Expect(managedSecret.Data).To(HaveKey("AXONOPS_DB_PASSWORD"))
			Expect(string(managedSecret.Data["AXONOPS_DB_USER"])).NotTo(BeEmpty())
			Expect(string(managedSecret.Data["AXONOPS_DB_PASSWORD"])).NotTo(BeEmpty())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, managedSecret)).To(Succeed())
		})

		It("should exclude timeseries-tls volume when TimeSeries is external", func() {
			const resourceName = "external-timeseries-no-tls"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create a timeseries auth secret
			timeseriesSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "timeseries-cred-secret",
					Namespace: "default",
				},
				Data: map[string][]byte{
					"AXONOPS_DB_USER":     []byte("axonops"),
					"AXONOPS_DB_PASSWORD": []byte("cassandra123"),
				},
			}
			Expect(k8sClient.Create(ctx, timeseriesSecret)).To(Succeed())

			// Create AxonOpsPlatform with external timeseries and minimal server config
			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"cassandra-node1.example.com:9042"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "timeseries-cred-secret",
							},
						},
					},
					Server: &corev1alpha1.AxonServerComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
						},
						OrgName: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that Server StatefulSet has no timeseries-tls volume")
			serverSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-server",
				Namespace: "default",
			}, serverSts)
			Expect(err).NotTo(HaveOccurred())

			// Check that timeseries-tls volume is not present
			for _, vol := range serverSts.Spec.Template.Spec.Volumes {
				Expect(vol.Name).NotTo(Equal("timeseries-tls"))
			}

			// Check that timeseries-tls volume mount is not present
			for _, mount := range serverSts.Spec.Template.Spec.Containers[0].VolumeMounts {
				Expect(mount.Name).NotTo(Equal("timeseries-tls"))
			}

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, timeseriesSecret)).To(Succeed())
		})
	})

	Context("Startup dependency ordering", func() {
		It("should report isStatefulSetReady=false when StatefulSet does not exist", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			ready, err := reconciler.isStatefulSetReady(ctx, "default", "nonexistent-sts")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())
		})

		It("should report isStatefulSetReady=false when StatefulSet has no ready replicas", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Create a StatefulSet with 1 replica but 0 ready
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sts-not-ready",
					Namespace: "default",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			ready, err := reconciler.isStatefulSetReady(ctx, "default", "sts-not-ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeFalse())

			Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
		})

		It("should report isStatefulSetReady=true when all replicas are ready", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Create a StatefulSet
			sts := &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sts-ready",
					Namespace: "default",
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: ptr(int32(1)),
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "test-ready"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test-ready"}},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{{Name: "c", Image: "busybox"}},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, sts)).To(Succeed())

			// Patch status to simulate ready replicas
			sts.Status.ReadyReplicas = 1
			sts.Status.Replicas = 1
			Expect(k8sClient.Status().Update(ctx, sts)).To(Succeed())

			ready, err := reconciler.isStatefulSetReady(ctx, "default", "sts-ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).To(BeTrue())

			Expect(k8sClient.Delete(ctx, sts)).To(Succeed())
		})

		It("should treat disabled components as ready", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			server := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-test", Namespace: "default"},
				Spec:       corev1alpha1.AxonOpsPlatformSpec{
					// TimeSeries and Search are nil (disabled)
				},
			}

			tsReady, tsReason := reconciler.isComponentReady(ctx, server, componentTimeseries)
			Expect(tsReady).To(BeTrue())
			Expect(tsReason).To(BeEmpty())

			searchReady, searchReason := reconciler.isComponentReady(ctx, server, componentSearch)
			Expect(searchReady).To(BeTrue())
			Expect(searchReason).To(BeEmpty())

			serverReady, serverReason := reconciler.isComponentReady(ctx, server, componentServer)
			Expect(serverReady).To(BeTrue())
			Expect(serverReason).To(BeEmpty())
		})

		It("should treat external components as ready", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			server := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-external-test", Namespace: "default"},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"cassandra:9042"},
							},
						},
					},
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://elastic:9200"},
							},
						},
					},
				},
			}

			tsReady, _ := reconciler.isComponentReady(ctx, server, componentTimeseries)
			Expect(tsReady).To(BeTrue())

			searchReady, _ := reconciler.isComponentReady(ctx, server, componentSearch)
			Expect(searchReady).To(BeTrue())
		})

		It("should report internal component as not ready when StatefulSet does not exist", func() {
			ctx := context.Background()
			reconciler := &AxonOpsPlatformReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			server := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-internal-test", Namespace: "default"},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							// No external hosts = internal
						},
					},
				},
			}

			ready, reason := reconciler.isComponentReady(ctx, server, componentTimeseries)
			Expect(ready).To(BeFalse())
			Expect(reason).To(ContainSubstring("Waiting for timeseries to become ready"))
		})

		// Note: full reconcile integration test for internal database blocking requires
		// cert-manager CRDs (not available in envtest). The gate logic is validated by
		// the isComponentReady/isStatefulSetReady unit tests above and by the
		// "should allow Server creation when databases are external" integration test below.
		// E2E tests against a Kind cluster with cert-manager will cover the full scenario.

		It("should allow Server creation when databases are external", func() {
			const resourceName = "dep-external-pass-test"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create auth secrets for external components
			tsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-ts-secret", Namespace: "default"},
				Data: map[string][]byte{
					"AXONOPS_DB_USER":     []byte("user"),
					"AXONOPS_DB_PASSWORD": []byte("pass"),
				},
			}
			searchSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "dep-search-secret", Namespace: "default"},
				Data: map[string][]byte{
					"AXONOPS_SEARCH_USER":     []byte("user"),
					"AXONOPS_SEARCH_PASSWORD": []byte("pass"),
				},
			}
			Expect(k8sClient.Create(ctx, tsSecret)).To(Succeed())
			Expect(k8sClient.Create(ctx, searchSecret)).To(Succeed())

			resource := &corev1alpha1.AxonOpsPlatform{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsPlatformSpec{
					TimeSeries: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"cassandra:9042"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "dep-ts-secret",
							},
						},
					},
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://elastic:9200"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								SecretRef: "dep-search-secret",
							},
						},
					},
					Server: &corev1alpha1.AxonServerComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: ptr(true),
						},
						OrgName: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling — external databases should be considered ready")
			controllerReconciler := &AxonOpsPlatformReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				ClusterIssuerName: "axonops-selfsigned",
				RESTMapper:        &mockRESTMapper{delegate: k8sClient.RESTMapper()},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Server StatefulSet WAS created (gate passed)")
			serverSts := &appsv1.StatefulSet{}
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-server",
				Namespace: "default",
			}, serverSts)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, tsSecret)).To(Succeed())
			Expect(k8sClient.Delete(ctx, searchSecret)).To(Succeed())
		})
	})
})
