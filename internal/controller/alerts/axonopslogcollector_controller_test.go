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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

var _ = Describe("AxonOpsLogCollector Controller", func() {
	const connName = "logcollector-conn"

	newLogCollectorCR := func(name, filename string) *alertsv1alpha1.AxonOpsLogCollector {
		return &alertsv1alpha1.AxonOpsLogCollector{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: alertsv1alpha1.AxonOpsLogCollectorSpec{
				ConnectionRef: connName,
				ClusterName:   testClusterName,
				ClusterType:   testClusterType,
				Name:          "Test Collector",
				Filename:      filename,
				Interval:      "5s",
				Timeout:       "1m",
				DateFormat:    "yyyy-MM-dd HH:mm:ss,SSS",
			},
		}
	}

	Context("Reconcile_Create_BasicCollector", func() {
		It("should create a log collector and set Ready", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`[]`))
				case http.MethodPut:
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-create-test", "/var/log/cassandra/gc.log")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: add finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, logCollectorFinalizerName)).To(BeTrue())

			// Second reconcile: sync with API
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedUUID).NotTo(BeEmpty())
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Create_ReusesExistingUUID", func() {
		It("should reuse UUID when collector with same filename exists", func() {
			existingUUID := "existing-uuid-12345"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					collectors := []axonops.LogCollector{
						{UUID: existingUUID, Filename: "/var/log/cassandra/gc.log", Name: "Old Name"},
					}
					data, _ := json.Marshal(collectors)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(data)
				case http.MethodPut:
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connReuse := connName + "-reuse"
			cleanup := createTestConnectionAndSecret(ctx, connReuse, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-reuse-uuid", "/var/log/cassandra/gc.log")
			cr.Spec.ConnectionRef = connReuse
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedUUID).To(Equal(existingUUID))
		})
	})

	Context("Reconcile_Create_PreservesOtherCollectors", func() {
		It("should preserve other collectors in the PUT payload", func() {
			var putPayload []axonops.LogCollector
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					collectors := []axonops.LogCollector{
						{UUID: "uuid-1", Filename: "/var/log/cassandra/gc.log", Name: "GC"},
						{UUID: "uuid-2", Filename: "/var/log/cassandra/sys.log", Name: "Sys"},
					}
					data, _ := json.Marshal(collectors)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(data)
				case http.MethodPut:
					body, _ := io.ReadAll(r.Body)
					formData, _ := url.ParseQuery(string(body))
					_ = json.Unmarshal([]byte(formData.Get("addlogs")), &putPayload)
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connPreserve := connName + "-preserve"
			cleanup := createTestConnectionAndSecret(ctx, connPreserve, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-preserve", "/var/log/cassandra/gc.log")
			cr.Spec.ConnectionRef = connPreserve
			cr.Spec.Name = "Updated GC"
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(putPayload).To(HaveLen(2))
			var gcFound, sysFound bool
			for _, lc := range putPayload {
				if lc.Filename == "/var/log/cassandra/gc.log" {
					gcFound = true
					Expect(lc.Name).To(Equal("Updated GC"))
					Expect(lc.UUID).To(Equal("uuid-1"))
				}
				if lc.Filename == "/var/log/cassandra/sys.log" {
					sysFound = true
					Expect(lc.Name).To(Equal("Sys"))
				}
			}
			Expect(gcFound).To(BeTrue())
			Expect(sysFound).To(BeTrue())
		})
	})

	Context("Reconcile_Create_PutUsesFormEncoding", func() {
		It("should use form-encoded body with addlogs field", func() {
			var contentType string
			var bodyHasAddlogs bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`[]`))
				case http.MethodPut:
					contentType = r.Header.Get("Content-Type")
					body, _ := io.ReadAll(r.Body)
					formData, err := url.ParseQuery(string(body))
					bodyHasAddlogs = err == nil && formData.Get("addlogs") != ""
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connForm := connName + "-form"
			cleanup := createTestConnectionAndSecret(ctx, connForm, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-form-test", "/var/log/cassandra/form.log")
			cr.Spec.ConnectionRef = connForm
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			Expect(contentType).To(Equal("application/x-www-form-urlencoded"))
			Expect(bodyHasAddlogs).To(BeTrue())
		})
	})

	Context("Reconcile_Idempotent_NoApiCallWhenSynced", func() {
		It("should not call API when already synced", func() {
			var apiCallCount atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				apiCallCount.Add(1)
				switch r.Method {
				case http.MethodGet:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`[]`))
				case http.MethodPut:
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connIdem := connName + "-idem"
			cleanup := createTestConnectionAndSecret(ctx, connIdem, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-idempotent", "/var/log/cassandra/idem.log")
			cr.Spec.ConnectionRef = connIdem
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile: finalizer
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			// Second reconcile: sync
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			callsAfterSync := apiCallCount.Load()

			// Third reconcile: should be idempotent
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(apiCallCount.Load()).To(Equal(callsAfterSync))
		})
	})

	Context("Reconcile_Delete_RemovesCollector", func() {
		It("should remove the collector and finalizer on deletion", func() {
			var putCalled atomic.Bool
			var putPayload []axonops.LogCollector
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					collectors := []axonops.LogCollector{
						{UUID: "del-uuid", Filename: "/var/log/cassandra/delete.log", Name: "Delete Me"},
						{UUID: "keep-uuid", Filename: "/var/log/cassandra/keep.log", Name: "Keep Me"},
					}
					data, _ := json.Marshal(collectors)
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(data)
				case http.MethodPut:
					putCalled.Store(true)
					body, _ := io.ReadAll(r.Body)
					formData, _ := url.ParseQuery(string(body))
					_ = json.Unmarshal([]byte(formData.Get("addlogs")), &putPayload)
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-delete-test", "/var/log/cassandra/delete.log")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Add finalizer + sync
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})

			// Delete
			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())
			putCalled.Store(false)

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(putCalled.Load()).To(BeTrue())
			Expect(putPayload).To(HaveLen(1))
			Expect(putPayload[0].Filename).To(Equal("/var/log/cassandra/keep.log"))

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Failed condition", func() {
			cr := newLogCollectorCR("lc-no-conn-test", "/var/log/cassandra/noconn.log")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, logCollectorFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First: add finalizer
			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			// Second: connection fails
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("FailedToResolveConnection"))
		})
	})

	Context("Reconcile_APIError_GetFails", func() {
		It("should set Failed condition on GET error", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			connGetErr := connName + "-geterr"
			cleanup := createTestConnectionAndSecret(ctx, connGetErr, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-get-err-test", "/var/log/cassandra/geterr.log")
			cr.Spec.ConnectionRef = connGetErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, logCollectorFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("SyncFailed"))
		})
	})

	Context("Reconcile_APIError_PutFails", func() {
		It("should set Failed condition on PUT error", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`[]`))
				case http.MethodPut:
					w.WriteHeader(http.StatusInternalServerError)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connPutErr := connName + "-puterr"
			cleanup := createTestConnectionAndSecret(ctx, connPutErr, server)
			defer cleanup()

			cr := newLogCollectorCR("lc-put-err-test", "/var/log/cassandra/puterr.log")
			cr.Spec.ConnectionRef = connPutErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, logCollectorFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsLogCollectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("SyncFailed"))
		})
	})
})
