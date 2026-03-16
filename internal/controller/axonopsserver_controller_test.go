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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

var _ = Describe("AxonOpsServer Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		axonopsserver := &corev1alpha1.AxonOpsServer{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind AxonOpsServer")
			err := k8sClient.Get(ctx, typeNamespacedName, axonopsserver)
			if err != nil && errors.IsNotFound(err) {
				resource := &corev1alpha1.AxonOpsServer{
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
			resource := &corev1alpha1.AxonOpsServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance AxonOpsServer")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &AxonOpsServerReconciler{
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

			// Create AxonOpsServer with external search configured
			resource := &corev1alpha1.AxonOpsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsServerSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: true,
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
			controllerReconciler := &AxonOpsServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
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
			updatedResource := &corev1alpha1.AxonOpsServer{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedResource)).To(Succeed())
			cond := meta.FindStatusCondition(updatedResource.Status.Conditions, "SearchMode")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal("External"))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			Expect(k8sClient.Delete(ctx, searchSecret)).To(Succeed())
		})

		It("should set condition when external Search credentials are missing", func() {
			const resourceName = "external-search-no-auth"
			ctx := context.Background()
			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			// Create AxonOpsServer with external search but no credentials
			resource := &corev1alpha1.AxonOpsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsServerSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: true,
							External: corev1alpha1.AxonExternalConfig{
								Hosts: []string{"https://opensearch.example.com:9200"},
							},
							Authentication: corev1alpha1.AxonAuthentication{
								// No SecretRef or Username - missing credentials
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource with missing Search credentials")
			controllerReconciler := &AxonOpsServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying that SearchReady condition indicates missing credentials")
			updatedResource := &corev1alpha1.AxonOpsServer{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updatedResource)).To(Succeed())
			cond := meta.FindStatusCondition(updatedResource.Status.Conditions, "SearchReady")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("MissingCredentials"))

			By("Cleanup")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
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

			// Create AxonOpsServer with external search and minimal server config
			resource := &corev1alpha1.AxonOpsServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: corev1alpha1.AxonOpsServerSpec{
					Search: &corev1alpha1.AxonDbComponent{
						AxonBaseComponent: corev1alpha1.AxonBaseComponent{
							Enabled: true,
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
							Enabled: true,
						},
						OrgName: "test-org",
					},
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			By("Reconciling the resource")
			controllerReconciler := &AxonOpsServerReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
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
})
