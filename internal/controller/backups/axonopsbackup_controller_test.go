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

package backups

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	backupsv1alpha1 "github.com/axonops/axonops-operator/api/backups/v1alpha1"
	"github.com/axonops/axonops-operator/internal/axonops"
)

var _ = Describe("AxonOpsBackup Controller", func() {
	const connName = "backup-conn"

	newBackupCR := func(name string) *backupsv1alpha1.AxonOpsBackup {
		return &backupsv1alpha1.AxonOpsBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: testNamespace,
			},
			Spec: backupsv1alpha1.AxonOpsBackupSpec{
				ConnectionRef:      connName,
				ClusterName:        testClusterName,
				ClusterType:        testClusterType,
				Tag:                "test-daily",
				Datacenters:        []string{"dc1"},
				LocalRetention:     "10d",
				ScheduleExpression: "0 1 * * *",
			},
		}
	}

	// newMockServer returns a TLS mock server handling backup API endpoints.
	newMockServer := func(getStatus, postStatus, deleteStatus int) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/api/v1/cassandraScheduleSnapshot/") && r.Method == http.MethodGet:
				if getStatus != http.StatusOK {
					w.WriteHeader(getStatus)
					return
				}
				resp := axonops.ScheduledSnapshotResponse{}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)

			case strings.Contains(r.URL.Path, "/api/v1/cassandraSnapshot/") && r.Method == http.MethodPost:
				w.WriteHeader(postStatus)

			case strings.Contains(r.URL.Path, "/api/v1/cassandraScheduleSnapshot/") && r.Method == http.MethodDelete:
				w.WriteHeader(deleteStatus)

			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	Context("Reconcile_Create_LocalBackup_Success", func() {
		It("should create a local backup and set Ready status", func() {
			server := newMockServer(http.StatusOK, http.StatusOK, http.StatusNoContent)
			defer server.Close()
			cleanup := createTestConnectionAndSecret(ctx, connName, server)
			defer cleanup()

			cr := newBackupCR("backup-create-test")
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(cr, backupFinalizerName)).To(BeTrue())

			// Syncs with API
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			Expect(cr.Status.SyncedBackupID).NotTo(BeEmpty())
			Expect(cr.Status.LastSyncTime).NotTo(BeNil())

			readyCond := meta.FindStatusCondition(cr.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Synced"))
		})
	})

	Context("Reconcile_Create_S3Backup_WithSecret", func() {
		It("should create an S3 backup with credentials from Secret", func() {
			var receivedBody string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/api/v1/cassandraSnapshot/"):
					bodyBytes, _ := io.ReadAll(r.Body)
					receivedBody = string(bodyBytes)
					w.WriteHeader(http.StatusOK)
				case r.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(axonops.ScheduledSnapshotResponse{})
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			connS3 := connName + "-s3"
			cleanup := createTestConnectionAndSecret(ctx, connS3, server)
			defer cleanup()

			// Create S3 credentials Secret
			s3Secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "s3-creds", Namespace: testNamespace},
				Data: map[string][]byte{
					"access_key_id":     []byte("AKIATEST"),
					"secret_access_key": []byte("secrettest"),
				},
			}
			Expect(k8sClient.Create(ctx, s3Secret)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, s3Secret)
			}()

			cr := newBackupCR("backup-s3-test")
			cr.Spec.ConnectionRef = connS3
			cr.Spec.Remote = &backupsv1alpha1.RemoteBackupConfig{
				Type: "s3",
				Path: "/backups/test",
				S3: &backupsv1alpha1.BackupS3Config{
					Region:         "eu-west-1",
					CredentialsRef: "s3-creds",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).To(ContainSubstring(`"Remote":true`))
			Expect(receivedBody).To(ContainSubstring("access_key_id = AKIATEST"))
			Expect(receivedBody).To(ContainSubstring("env_auth = false"))
		})
	})

	Context("Reconcile_Create_S3Backup_InlineCredentials", func() {
		It("should use inline S3 credentials when no SecretRef", func() {
			var receivedBody string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					bodyBytes, _ := io.ReadAll(r.Body)
					receivedBody = string(bodyBytes)
					w.WriteHeader(http.StatusOK)
				} else {
					_ = json.NewEncoder(w).Encode(axonops.ScheduledSnapshotResponse{})
				}
			}))
			defer server.Close()
			connInline := connName + "-inline"
			cleanup := createTestConnectionAndSecret(ctx, connInline, server)
			defer cleanup()

			cr := newBackupCR("backup-s3-inline-test")
			cr.Spec.ConnectionRef = connInline
			cr.Spec.Remote = &backupsv1alpha1.RemoteBackupConfig{
				Type: "s3",
				Path: "/backups/inline",
				S3: &backupsv1alpha1.BackupS3Config{
					Region:          "us-east-1",
					AccessKeyID:     "INLINE_KEY",
					SecretAccessKey: "INLINE_SECRET",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(ctx, cr)
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedBody).To(ContainSubstring("access_key_id = INLINE_KEY"))
			Expect(receivedBody).To(ContainSubstring("secret_access_key = INLINE_SECRET"))
			Expect(receivedBody).To(ContainSubstring("env_auth = false"))
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
				default:
					_ = json.NewEncoder(w).Encode(axonops.ScheduledSnapshotResponse{})
				}
			}))
			defer server.Close()
			connDel := connName + "-del"
			cleanup := createTestConnectionAndSecret(ctx, connDel, server)
			defer cleanup()

			cr := newBackupCR("backup-delete-test")
			cr.Spec.ConnectionRef = connDel
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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

	Context("Reconcile_Delete_API404", func() {
		It("should remove finalizer even when API returns 404", func() {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					w.WriteHeader(http.StatusNotFound)
				case http.MethodPost:
					w.WriteHeader(http.StatusOK)
				default:
					_ = json.NewEncoder(w).Encode(axonops.ScheduledSnapshotResponse{})
				}
			}))
			defer server.Close()
			conn404 := connName + "-404"
			cleanup := createTestConnectionAndSecret(ctx, conn404, server)
			defer cleanup()

			cr := newBackupCR("backup-404-test")
			cr.Spec.ConnectionRef = conn404
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
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

	Context("Reconcile_ConnectionNotFound", func() {
		It("should set Failed condition when connection is missing", func() {
			cr := newBackupCR("backup-no-conn-test")
			cr.Spec.ConnectionRef = nonexistentConnName
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, backupFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

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
			server := newMockServer(http.StatusOK, http.StatusInternalServerError, http.StatusNoContent)
			defer server.Close()
			connErr := connName + "-err"
			cleanup := createTestConnectionAndSecret(ctx, connErr, server)
			defer cleanup()

			cr := newBackupCR("backup-api-error-test")
			cr.Spec.ConnectionRef = connErr
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, backupFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

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

	Context("Reconcile_TablesKeyspacesMutuallyExclusive", func() {
		It("should set Failed condition when both tables and keyspaces specified", func() {
			server := newMockServer(http.StatusOK, http.StatusOK, http.StatusNoContent)
			defer server.Close()
			connME := connName + "-me"
			cleanup := createTestConnectionAndSecret(ctx, connME, server)
			defer cleanup()

			cr := newBackupCR("backup-mutual-excl-test")
			cr.Spec.ConnectionRef = connME
			cr.Spec.Tables = []string{"ks.table1"}
			cr.Spec.Keyspaces = []string{"ks"}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
			defer func() {
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: testNamespace}, cr); err == nil {
					controllerutil.RemoveFinalizer(cr, backupFinalizerName)
					_ = k8sClient.Update(ctx, cr)
					_ = k8sClient.Delete(ctx, cr)
				}
			}()

			nn := types.NamespacedName{Name: cr.Name, Namespace: testNamespace}
			reconciler := &AxonOpsBackupReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			// Adds finalizer
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Validation fails
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, nn, cr)).To(Succeed())
			failedCond := meta.FindStatusCondition(cr.Status.Conditions, "Failed")
			Expect(failedCond).NotTo(BeNil())
			Expect(failedCond.Reason).To(Equal("ValidationError"))
		})
	})
})
