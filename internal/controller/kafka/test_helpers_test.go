package kafka

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
	testNamespace       = "default"
	nonexistentConnName = "nonexistent-conn"
)

func createTestConnectionAndSecret(ctx context.Context, connName string, server *httptest.Server) func() {
	u, err := url.Parse(server.URL)
	Expect(err).NotTo(HaveOccurred())

	secretName := connName + "-api-key"

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
		Data:       map[string][]byte{"api_key": []byte("test-api-key")},
	}
	Expect(k8sClient.Create(ctx, secret)).To(Succeed())

	conn := &corev1alpha1.AxonOpsConnection{
		ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
		Spec: corev1alpha1.AxonOpsConnectionSpec{
			OrgID:         testOrgID,
			APIKeyRef:     corev1alpha1.AxonOpsSecretKeyRef{Name: secretName},
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
