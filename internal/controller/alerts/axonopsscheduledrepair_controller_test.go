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

package alerts

import (
	"encoding/json"
	"io"
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

var _ = Describe("AxonOpsScheduledRepair Controller", func() {
	const connName = "repair-conn"

	newRepairCR := func(name string) *alertsv1alpha1.AxonOpsScheduledRepair {
		return &alertsv1alpha1.AxonOpsScheduledRepair{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: alertsv1alpha1.AxonOpsScheduledRepairSpec{
				ConnectionRef:      connName,
				ClusterName:        testClusterName,
				ClusterType:        testClusterType,
				Tag:                "test-repair",
				ScheduleExpression: "0 0 1 * *",
			},
		}
	}

	// newMockServer handles repair API endpoints.
	// After POST, GET returns a repair with the tag so the controller can find the ID.
	newMockServer := func(postStatus, deleteStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v1/repair/") && r.Method == http.MethodGet:
				resp := axonops.ScheduledRepairsResponse{
					Repairs: []axonops.ScheduledRepairEntry{
						{
							ID: "mock-repair-id",
							Params: []axonops.ScheduledRepairParams{
								{Tag: "test-repair", ScheduleExpr: "0 0 1 * *"},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)

			case strings.Contains(r.URL.Path, "/api/v1/addrepair/") && r.Method == http.MethodPost:
				w.WriteHeader(postStatus)

			case strings.Contains(r.URL.Path, "/api/v1/cassandrascheduledrepair/") && r.Method == http.MethodDelete:
				w.WriteHeader(deleteStatus)

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create a scheduled repair and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusNoContent)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newRepairCR("repair-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, scheduledRepairFinalizerName)).To(BeTrue())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedRepairID).To(Equal("mock-repair-id"))
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Create_FullConfig", func() {
		It("should send all fields in the API payload", func() {
			var receivedBody string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPost:
					bodyBytes, _ := io.ReadAll(r.Body)
					receivedBody = string(bodyBytes)
					w.WriteHeader(http.StatusOK)
				case http.MethodGet:
					resp := axonops.ScheduledRepairsResponse{
						Repairs: []axonops.ScheduledRepairEntry{
							{ID: "full-id", Params: []axonops.ScheduledRepairParams{{Tag: "full-config"}}},
						},
					}
					_ = json.NewEncoder(w).Encode(resp)
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			connFull := connName + "-full"
			cleanup := createTestConnectionAndSecret(ctx, connFull, server)
			defer cleanup()

			cr := newRepairCR("repair-full-test")
			cr.Spec.ConnectionRef = connFull
			cr.Spec.Tag = "full-config"
			cr.Spec.Keyspace = "analytics"
			cr.Spec.BlacklistedTables = []string{"large_events"}
			cr.Spec.SpecificDataCenters = []string{"dc1"}
			cr.Spec.Parallelism = "DC-Aware"
			cr.Spec.Segmented = true
			cr.Spec.SegmentsPerNode = 4
			cr.Spec.Incremental = true
			cr.Spec.JobThreads = 2
			cr.Spec.OptimiseStreams = true
			cr.Spec.SkipPaxos = true
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).To(ContainSubstring(`"keyspace":"analytics"`))
			Expect(receivedBody).To(ContainSubstring(`"parallelism":"DC-Aware"`))
			Expect(receivedBody).To(ContainSubstring(`"segmented":true`))
			Expect(receivedBody).To(ContainSubstring(`"segmentsPerNode":4`))
			Expect(receivedBody).To(ContainSubstring(`"incremental":true`))
			Expect(receivedBody).To(ContainSubstring(`"jobThreads":2`))
			Expect(receivedBody).To(ContainSubstring(`"optimiseStreams":true`))
			Expect(receivedBody).To(ContainSubstring(`"paxos":"Skip Paxos"`))
			Expect(receivedBody).To(ContainSubstring(`"skipPaxos":true`))
		})
	})

	Context("Reconcile_Delete_WithFinalizer", func() {
		It("should call API delete and remove finalizer", func() {
			var deleteCalled atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					deleteCalled.Store(true)
					w.WriteHeader(http.StatusNoContent)
				case http.MethodPost:
					w.WriteHeader(http.StatusOK)
				case http.MethodGet:
					resp := axonops.ScheduledRepairsResponse{
						Repairs: []axonops.ScheduledRepairEntry{
							{ID: "del-id", Params: []axonops.ScheduledRepairParams{{Tag: "test-repair"}}},
						},
					}
					_ = json.NewEncoder(w).Encode(resp)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newRepairCR("repair-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, cr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(deleteCalled.Load()).To(BeTrue())

			err = k8sClient.Get(ctx, nn, cr)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Failed condition when connection is missing", func() {
			cr := newRepairCR("repair-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, scheduledRepairFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("FailedToResolveConnection"))
		})
	})

	Context("Reconcile_APIError", func() {
		It("should set Failed condition when API returns error", func() {
			server := newMockServer(http.StatusInternalServerError, http.StatusNoContent)
			defer server.Close()
			connErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connErr, server)
			defer cleanup()

			cr := newRepairCR("repair-api-error-test")
			cr.Spec.ConnectionRef = connErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, scheduledRepairFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("SyncFailed"))
		})
	})

	Context("Reconcile_PaxosMutuallyExclusive", func() {
		It("should set Failed when both skipPaxos and paxosOnly are true", func() {
			server := newMockServer(http.StatusOK, http.StatusNoContent)
			defer server.Close()
			connME := connName + "-paxos"
			cleanup := createTestConnectionAndSecret(ctx, connME, server)
			defer cleanup()

			cr := newRepairCR("repair-paxos-test")
			cr.Spec.ConnectionRef = connME
			cr.Spec.SkipPaxos = true
			cr.Spec.PaxosOnly = true
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, scheduledRepairFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsScheduledRepairReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("ValidationError"))
		})
	})
})
