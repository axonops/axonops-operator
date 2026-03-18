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

var _ = Describe("AxonOpsMetricAlert Controller", func() {
	const connName = "metric-alert-conn"
	// newMetricAlertCR creates a metric alert CR with the given name.
	newMetricAlertCR := func(name string) *alertsv1alpha1.AxonOpsMetricAlert {
		return &alertsv1alpha1.AxonOpsMetricAlert{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: alertsv1alpha1.AxonOpsMetricAlertSpec{
				ConnectionRef: connName,
				ClusterName:   testClusterName,
				ClusterType:   testClusterType,
				Name:          "Test Alert",
				Operator:      ">",
				WarningValue:  50,
				CriticalValue: 100,
				Duration:      "15m",
				Dashboard:     "Test Dashboard",
				Chart:         "Test Chart",
				Metric:        "test_metric",
			},
		}
	}

	// newMockServer returns a TLS mock server that handles dashboard and alert-rules endpoints.
	newMockServer := func(dashboardStatus, alertStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v1/dashboardtemplate/"):
				if dashboardStatus != http.StatusOK {
					w.WriteHeader(dashboardStatus)
					return
				}
				resp := axonops.DashboardTemplateResponse{
					Dashboards: []axonops.Dashboard{
						{
							UUID: "dash-uuid",
							Name: "Test Dashboard",
							Panels: []axonops.DashboardPanel{
								{UUID: "panel-uuid-123", Title: "Test Chart"},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)

			case strings.Contains(r.URL.Path, "/api/v1/alert-rules/"):
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if alertStatus != http.StatusOK && alertStatus != http.StatusCreated {
					w.WriteHeader(alertStatus)
					_, _ = w.Write([]byte(`{"error":"test error"}`))
					return
				}
				var rule axonops.MetricAlertRule
				_ = json.NewDecoder(r.Body).Decode(&rule)
				if rule.ID == "" {
					rule.ID = "generated-alert-id"
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(rule)

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create the alert and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusCreated)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newMetricAlertCR("metric-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify finalizer was added
			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, finalizerName)).To(BeTrue())

			// Second reconcile: syncs with API
			result, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Verify status
			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedAlertID).NotTo(BeEmpty())
			Expect(cr.Status.CorrelationID).To(Equal("panel-uuid-123"))
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Synced"))
		})
	})

	Context("Reconcile_Update_Success", func() {
		It("should update the alert when spec changes", func() {
			server := newMockServer(http.StatusOK, http.StatusCreated)
			defer server.Close()
			connNameUpd := connName + "-upd"
			cleanup := createTestConnectionAndSecret(ctx, connNameUpd, server)
			defer cleanup()

			cr := newMetricAlertCR("metric-update-test")
			cr.Spec.ConnectionRef = connNameUpd
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: initial create
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			originalAlertID := cr.Status.SyncedAlertID
			Expect(originalAlertID).NotTo(BeEmpty())

			// Update the CR spec
			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			cr.Spec.WarningValue = 75
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			// Third reconcile: updates the alert (generation changed)
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
			var deleteCalled atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/api/v1/dashboardtemplate/"):
					resp := axonops.DashboardTemplateResponse{
						Dashboards: []axonops.Dashboard{
							{Name: "Test Dashboard", Panels: []axonops.DashboardPanel{{UUID: "p-uuid", Title: "Test Chart"}}},
						},
					}
					_ = json.NewEncoder(w).Encode(resp)
				case strings.Contains(r.URL.Path, "/api/v1/alert-rules/") && r.Method == http.MethodDelete:
					deleteCalled.Store(true)
					w.WriteHeader(http.StatusNoContent)
				case strings.Contains(r.URL.Path, "/api/v1/alert-rules/"):
					rule := axonops.MetricAlertRule{ID: "delete-test-id"}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(rule)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			connNameDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connNameDel, server)
			defer cleanup()

			cr := newMetricAlertCR("metric-delete-test")
			cr.Spec.ConnectionRef = connNameDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Reconcile to add finalizer + sync
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedAlertID).NotTo(BeEmpty())

			// Delete the CR
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			// Reconcile handles deletion
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(deleteCalled.Load()).To(BeTrue())

			// CR should be gone (finalizer removed)
			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Failed condition when connection is missing", func() {
			cr := newMetricAlertCR("metric-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				// Remove finalizer to allow cleanup
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, finalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: connection resolution fails
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(failedCond.Reason).To(Equal("FailedToResolveConnection"))
		})
	})

	Context("Reconcile_APIError", func() {
		It("should set Failed condition when API returns error", func() {
			server := newMockServer(http.StatusOK, http.StatusInternalServerError)
			defer server.Close()
			connNameErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connNameErr, server)
			defer cleanup()

			cr := newMetricAlertCR("metric-api-error-test")
			cr.Spec.ConnectionRef = connNameErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, finalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Adds finalizer
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// API error on create
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero()) // 500 is retryable

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(failedCond.Reason).To(Equal("SyncFailed"))
		})
	})

	Context("Reconcile_API404OnDelete", func() {
		It("should remove finalizer even when API returns 404 on delete", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.Contains(r.URL.Path, "/api/v1/dashboardtemplate/"):
					resp := axonops.DashboardTemplateResponse{
						Dashboards: []axonops.Dashboard{
							{Name: "Test Dashboard", Panels: []axonops.DashboardPanel{{UUID: "p", Title: "Test Chart"}}},
						},
					}
					_ = json.NewEncoder(w).Encode(resp)
				case strings.Contains(r.URL.Path, "/api/v1/alert-rules/") && r.Method == http.MethodDelete:
					// Return 404 — alert already deleted
					w.WriteHeader(http.StatusNotFound)
				case strings.Contains(r.URL.Path, "/api/v1/alert-rules/"):
					rule := axonops.MetricAlertRule{ID: "id-404-test"}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(rule)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			connName404 := connName + "-404"
			cleanup := createTestConnectionAndSecret(ctx, connName404, server)
			defer cleanup()

			cr := newMetricAlertCR("metric-404-delete-test")
			cr.Spec.ConnectionRef = connName404
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsMetricAlertReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Create + sync
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete CR
			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			// Reconcile deletion — API returns 404 but finalizer should still be removed
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// CR should be fully deleted
			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})
})
