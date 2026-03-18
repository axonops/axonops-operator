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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	alertsv1alpha1 "github.com/axonops/axonops-operator/api/alerts/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

var _ = Describe("AxonOpsAlertRoute Controller", func() {
	const connName = "route-conn"
	newAlertRouteCR := func(name string) *alertsv1alpha1.AxonOpsAlertRoute {
		return &alertsv1alpha1.AxonOpsAlertRoute{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: alertsv1alpha1.AxonOpsAlertRouteSpec{
				ConnectionRef:   connName,
				ClusterName:     testClusterName,
				ClusterType:     testClusterType,
				IntegrationName: "test-slack",
				IntegrationType: "slack",
				Type:            "metrics",
				Severity:        "error",
			},
		}
	}

	// integrationsResponse returns a mock integrations response with a matching integration.
	integrationsResponse := func() axonops.IntegrationsResponse {
		return axonops.IntegrationsResponse{
			Definitions: []axonops.IntegrationDefinition{
				{
					ID:     "int-123",
					Type:   "slack",
					Params: map[string]string{"name": "test-slack"},
				},
			},
			Routings: []axonops.IntegrationRouting{},
		}
	}

	newMockServer := func(integrationsStatus, routeStatus, overrideStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v1/integrations-routing/"):
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(routeStatus)

			case strings.Contains(r.URL.Path, "/api/v1/integrations-override/"):
				w.WriteHeader(overrideStatus)

			case strings.Contains(r.URL.Path, "/api/v1/integrations/"):
				if integrationsStatus != http.StatusOK {
					w.WriteHeader(integrationsStatus)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(integrationsResponse())

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create the route and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusCreated, http.StatusOK)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newAlertRouteCR("route-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, alertRouteFinalizerName)).To(BeTrue())

			// Syncs route
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.IntegrationID).To(Equal("int-123"))
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("RouteSynced"))
		})
	})

	Context("Reconcile_Update_Success", func() {
		It("should update the route when spec changes", func() {
			var err error
			server := newMockServer(http.StatusOK, http.StatusCreated, http.StatusOK)
			defer server.Close()
			connUpd := connName + "-upd"
			cleanup := createTestConnectionAndSecret(ctx, connUpd, server)
			defer cleanup()

			cr := newAlertRouteCR("route-update-test")
			cr.Spec.ConnectionRef = connUpd
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Update severity
			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			cr.Spec.Severity = "warning"
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Delete_WithFinalizer", func() {
		It("should call API delete and remove finalizer", func() {
			var routeDeleteCalled atomic.Bool
			var err error
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/api/v1/integrations-routing/") && r.Method == http.MethodDelete:
					routeDeleteCalled.Store(true)
					w.WriteHeader(http.StatusNoContent)
				case strings.Contains(r.URL.Path, "/api/v1/integrations-routing/"):
					w.WriteHeader(http.StatusCreated)
				case strings.Contains(r.URL.Path, "/api/v1/integrations-override/"):
					w.WriteHeader(http.StatusOK)
				case strings.Contains(r.URL.Path, "/api/v1/integrations/"):
					_ = json.NewEncoder(w).Encode(integrationsResponse())
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newAlertRouteCR("route-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(routeDeleteCalled.Load()).To(BeTrue())

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Ready=False when connection is missing", func() {
			cr := newAlertRouteCR("route-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, alertRouteFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err1 := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err1).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionError"))
		})
	})

	Context("Reconcile_APIError", func() {
		It("should set Ready=False when API returns error", func() {
			var err error
			server := newMockServer(http.StatusInternalServerError, http.StatusOK, http.StatusOK)
			defer server.Close()
			connErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connErr, server)
			defer cleanup()

			cr := newAlertRouteCR("route-api-error-test")
			cr.Spec.ConnectionRef = connErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, alertRouteFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("APIError"))
		})
	})

	Context("Reconcile_API404OnDelete", func() {
		It("should remove finalizer even when integration not found on delete", func() {
			var callCount atomic.Int32
			var err error
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/api/v1/integrations-routing/"):
					w.WriteHeader(http.StatusCreated)
				case strings.Contains(r.URL.Path, "/api/v1/integrations-override/"):
					w.WriteHeader(http.StatusOK)
				case strings.Contains(r.URL.Path, "/api/v1/integrations/"):
					callCount.Add(1)
					if callCount.Load() > 2 {
						// On deletion, return empty integrations (integration not found)
						_ = json.NewEncoder(w).Encode(axonops.IntegrationsResponse{})
						return
					}
					_ = json.NewEncoder(w).Encode(integrationsResponse())
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			conn404 := connName + "-404"
			cleanup := createTestConnectionAndSecret(ctx, conn404, server)
			defer cleanup()

			cr := newAlertRouteCR("route-404-delete-test")
			cr.Spec.ConnectionRef = conn404
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsAlertRouteReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})
})
