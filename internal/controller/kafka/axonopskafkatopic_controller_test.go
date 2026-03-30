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

package kafka

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kafkav1alpha1 "github.com/axonops/axonops-operator/api/kafka/v1alpha1"
)

var _ = Describe("AxonOpsKafkaTopic Controller", func() {
	const connName = "kafka-conn"

	newTopicCR := func(name string) *kafkav1alpha1.AxonOpsKafkaTopic {
		return &kafkav1alpha1.AxonOpsKafkaTopic{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: kafkav1alpha1.AxonOpsKafkaTopicSpec{
				ConnectionRef:     connName,
				ClusterName:       "test-kafka",
				Name:              "test-topic",
				Partitions:        3,
				ReplicationFactor: 2,
			},
		}
	}

	newMockServer := func(postStatus, putStatus, deleteStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(postStatus)
			case http.MethodPut:
				w.WriteHeader(putStatus)
			case http.MethodDelete:
				w.WriteHeader(deleteStatus)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create the topic and set Ready status", func() {
			server := newMockServer(http.StatusCreated, http.StatusNoContent, http.StatusNoContent)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newTopicCR("topic-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, kafkaTopicFinalizerName)).To(BeTrue())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.Synced).To(BeTrue())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Create_WithConfig", func() {
		It("should send config in the API payload", func() {
			var receivedBody string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					bodyBytes, _ := io.ReadAll(r.Body)
					receivedBody = string(bodyBytes)
				}
				w.WriteHeader(http.StatusCreated)
			}))
			defer server.Close()
			connCfg := connName + "-cfg"
			cleanup := createTestConnectionAndSecret(ctx, connCfg, server)
			defer cleanup()

			cr := newTopicCR("topic-config-test")
			cr.Spec.ConnectionRef = connCfg
			cr.Spec.Config = map[string]string{
				"cleanup.policy": "compact",
				"retention.ms":   "604800000",
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).To(ContainSubstring(`"topicName":"test-topic"`))
			Expect(receivedBody).To(ContainSubstring(`"partitionCount":3`))
			Expect(receivedBody).To(ContainSubstring("cleanup.policy"))
		})
	})

	Context("Reconcile_Delete_WithFinalizer", func() {
		It("should delete the topic and remove finalizer", func() {
			var deleteCalled atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					deleteCalled.Store(true)
					w.WriteHeader(http.StatusNoContent)
				case http.MethodPost:
					w.WriteHeader(http.StatusCreated)
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newTopicCR("topic-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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
		It("should set Failed condition", func() {
			cr := newTopicCR("topic-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, kafkaTopicFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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

	Context("Reconcile_Create_APIBodyError", func() {
		It("should set Failed condition and not mark Synced when API returns 200 with error body", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"error":"available kafka nodes not found"}`))
			}))
			defer server.Close()
			connBody := connName + "-body-err"
			cleanup := createTestConnectionAndSecret(ctx, connBody, server)
			defer cleanup()

			cr := newTopicCR("topic-body-error-test")
			cr.Spec.ConnectionRef = connBody
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, kafkaTopicFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// First reconcile adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile attempts create and hits body-level error
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.Synced).To(BeFalse())

			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("CreateFailed"))
			Expect(failedCond.Message).To(ContainSubstring("available kafka nodes not found"))

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).To(BeNil())
		})
	})

	Context("Reconcile_Create_ServerError_IncludesAPIMessage", func() {
		It("should include the API error message in the Failed condition when server returns 500", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"available kafka nodes not found"}`))
			}))
			defer server.Close()
			connMsg := connName + "-msg-err"
			cleanup := createTestConnectionAndSecret(ctx, connMsg, server)
			defer cleanup()

			cr := newTopicCR("topic-msg-error-test")
			cr.Spec.ConnectionRef = connMsg
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, kafkaTopicFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.Synced).To(BeFalse())

			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("CreateFailed"))
			Expect(failedCond.Message).To(ContainSubstring("available kafka nodes not found"))
		})
	})

	Context("Reconcile_Update_ConfigOnly", func() {
		It("should update config via PUT when topic already synced", func() {
			var putCalled atomic.Bool
			var putBody string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodPut:
					putCalled.Store(true)
					bodyBytes, _ := io.ReadAll(r.Body)
					putBody = string(bodyBytes)
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusCreated)
				}
			}))
			defer server.Close()
			connUpd := connName + "-upd"
			cleanup := createTestConnectionAndSecret(ctx, connUpd, server)
			defer cleanup()

			cr := newTopicCR("topic-update-test")
			cr.Spec.ConnectionRef = connUpd
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaTopicReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			cr.Spec.Config = map[string]string{"retention.ms": "86400000"}
			Expect(k8sClient.Update(ctx, cr)).To(Succeed())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(putCalled.Load()).To(BeTrue())
			Expect(putBody).To(ContainSubstring("retention.ms"))
			Expect(putBody).To(ContainSubstring(`"op":"SET"`))
		})
	})
})
