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

package kafka

import (
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

var _ = Describe("AxonOpsKafkaACL Controller", func() {
	const connName = "acl-conn"

	newACLCR := func(name string) *kafkav1alpha1.AxonOpsKafkaACL {
		return &kafkav1alpha1.AxonOpsKafkaACL{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: kafkav1alpha1.AxonOpsKafkaACLSpec{
				ConnectionRef:       connName,
				ClusterName:         "test-kafka",
				ResourceType:        "TOPIC",
				ResourceName:        "my-topic",
				ResourcePatternType: "LITERAL",
				Principal:           "User:producer-app",
				Host:                "*",
				Operation:           "WRITE",
				PermissionType:      "ALLOW",
			},
		}
	}

	newMockServer := func(postStatus, deleteStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				w.WriteHeader(postStatus)
			case http.MethodDelete:
				w.WriteHeader(deleteStatus)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_Success", func() {
		It("should create the ACL and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusNoContent)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newACLCR("acl-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() { _ = k8sClient.Delete(ctx, cr) }()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaACLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.Synced).To(BeTrue())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Reconcile_Delete_WithFinalizer", func() {
		It("should delete the ACL and remove finalizer", func() {
			var deleteCalled atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					deleteCalled.Store(true)
					w.WriteHeader(http.StatusNoContent)
				default:
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newACLCR("acl-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaACLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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
			cr := newACLCR("acl-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, kafkaACLFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaACLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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
		It("should set Failed condition on API error", func() {
			server := newMockServer(http.StatusInternalServerError, http.StatusNoContent)
			defer server.Close()
			connErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connErr, server)
			defer cleanup()

			cr := newACLCR("acl-api-error-test")
			cr.Spec.ConnectionRef = connErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, kafkaACLFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsKafkaACLReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("CreateFailed"))
		})
	})
})
