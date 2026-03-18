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

var _ = Describe("AxonOpsHealthcheckTCP Controller", func() {
	const connName = "tcp-hc-conn"

	ctx := context.Background()

	newTCPHealthcheckCR := func(name string) *alertsv1alpha1.AxonOpsHealthcheckTCP {
		return &alertsv1alpha1.AxonOpsHealthcheckTCP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: alertsv1alpha1.AxonOpsHealthcheckTCPSpec{
				ConnectionRef: connName,
				ClusterName:   testClusterName,
				ClusterType:   "kafka",
				Name:          "test-tcp-healthcheck",
				TCP:           "0.0.0.0:9092",
				Interval:      "1m",
				Timeout:       "30s",
			},
		}
	}

	emptyHealthchecksResp := func() axonops.HealthchecksResponse {
		return axonops.HealthchecksResponse{}
	}

	newMockServer := func(getStatus, putStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.URL.Path, "/api/v1/healthchecks/") {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			switch r.Method {
			case http.MethodGet:
				if getStatus != http.StatusOK {
					w.WriteHeader(getStatus)
					return
				}
				_ = json.NewEncoder(w).Encode(emptyHealthchecksResp())
			case http.MethodPut:
				w.WriteHeader(putStatus)
			default:
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create the healthcheck and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusOK)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newTCPHealthcheckCR("tcp-hc-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, tcpHealthcheckFinalizer)).To(BeTrue())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedHealthcheckID).NotTo(BeEmpty())
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Synced"))
		})
	})

	Context("Reconcile_Update_Success", func() {
		It("should update the healthcheck when spec changes", func() {
			server := newMockServer(http.StatusOK, http.StatusOK)
			defer server.Close()
			connUpd := connName + "-upd"
			cleanup := createTestConnectionAndSecret(ctx, connUpd, server)
			defer cleanup()

			cr := newTCPHealthcheckCR("tcp-hc-update-test")
			cr.Spec.ConnectionRef = connUpd
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			cr.Spec.TCP = "0.0.0.0:9093"
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Delete_WithFinalizer", func() {
		It("should remove healthcheck from API and remove finalizer", func() {
			var putCalled atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					_ = json.NewEncoder(w).Encode(emptyHealthchecksResp())
				case http.MethodPut:
					putCalled.Store(true)
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newTCPHealthcheckCR("tcp-hc-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(putCalled.Load()).To(BeTrue())

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Failed condition when connection is missing", func() {
			cr := newTCPHealthcheckCR("tcp-hc-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, tcpHealthcheckFinalizer)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(failedCond.Reason).To(Equal(ReasonConnectionError))
		})
	})

	Context("Reconcile_APIError", func() {
		It("should set Failed condition when API returns error", func() {
			server := newMockServer(http.StatusInternalServerError, http.StatusOK)
			defer server.Close()
			connErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connErr, server)
			defer cleanup()

			cr := newTCPHealthcheckCR("tcp-hc-api-error-test")
			cr.Spec.ConnectionRef = connErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, tcpHealthcheckFinalizer)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal(ReasonAPIError))
		})
	})

	Context("Reconcile_API404OnDelete", func() {
		It("should remove finalizer even when API returns 404 on delete", func() {
			var callCount atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					callCount.Add(1)
					if callCount.Load() > 1 {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(emptyHealthchecksResp())
				case http.MethodPut:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			conn404 := connName + "-404"
			cleanup := createTestConnectionAndSecret(ctx, conn404, server)
			defer cleanup()

			cr := newTCPHealthcheckCR("tcp-hc-404-delete-test")
			cr.Spec.ConnectionRef = conn404
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsHealthcheckTCPReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})
})
