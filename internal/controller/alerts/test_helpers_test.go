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

// Test helpers shared by all alert controller tests.
// Each controller test uses httptest.NewTLSServer to mock the AxonOps API,
// and creates an AxonOpsConnection + Secret in envtest that points to the mock server.

package alerts

import (
	"context"
	"net/http/httptest"
	"net/url"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

const (
	testOrgID           = "test-org"
	testClusterName     = "test-cluster"
	testClusterType     = "cassandra"
	testNamespace       = "default"
	nonexistentConnName = "nonexistent-conn"
)

// createTestConnectionAndSecret creates an AxonOpsConnection CR and its API key Secret
// that point to the given httptest.Server URL. Returns a cleanup function.
func createTestConnectionAndSecret(ctx context.Context, connName string, server *httptest.Server) func() {
	u, err := url.Parse(server.URL)
	Expect(err).NotTo(HaveOccurred())

	secretName := connName + "-api-key"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"api_key": []byte("test-api-key-value"),
		},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())

	conn := &corev1alpha1.AxonOpsConnection{
		ObjectMeta: metav1.ObjectMeta{
			Name:      connName,
			Namespace: testNamespace,
		},
		Spec: corev1alpha1.AxonOpsConnectionSpec{
			OrgID: testOrgID,
			APIKeyRef: corev1alpha1.AxonOpsSecretKeyRef{
				Name: secretName,
			},
			Host:          u.Host,
			TLSSkipVerify: true,
		},
	}
	Expect(k8sClient.Create(ctx, conn)).To(Succeed())

	return func() {
		_ = client.IgnoreNotFound(k8sClient.Delete(ctx, secret))
		_ = client.IgnoreNotFound(k8sClient.Delete(ctx, conn))
	}
}
