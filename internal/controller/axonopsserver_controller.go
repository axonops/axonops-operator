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

package controller

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

const (
	// Secret keys for database credentials
	searchSecretKeyUser     = "SEARCH_DB_USERNAME"
	searchSecretKeyPassword = "SEARCH_DB_PASSWORD"

	timeseriesSecretKeyUser     = "AXONOPS_DB_USER"
	timeseriesSecretKeyPassword = "AXONOPS_DB_PASSWORD"
	// Default username prefix for auto-generated credentials
	defaultUsernamePrefix = "axonops"
)

// AxonOpsServerReconciler reconciles a AxonOpsServer object
type AxonOpsServerReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ClusterIssuerName string
}

// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.axonops.com,resources=axonopsservers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the current state of the cluster closer to the desired state.
func (r *AxonOpsServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsServer CR
	server := &corev1alpha1.AxonOpsServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsServer", "name", req.NamespacedName)

	var timeSeriesSecretName, searchSecretName string
	var timeSeriesCertSecretName, searchCertSecretName string

	// Ensure TimeSeries authentication secret and certificate (if component is enabled)
	if r.isComponentEnabled(server.Spec.TimeSeries) {
		var err error
		timeSeriesSecretName, err = r.ensureAuthenticationSecret(ctx, server, "timeseries", server.Spec.TimeSeries.Authentication)
		if err != nil {
			log.Error(err, "Failed to ensure TimeSeries authentication secret")
			return ctrl.Result{}, err
		}

		// Create TLS certificate for TimeSeries
		timeSeriesCertSecretName, err = r.ensureTLSCertificate(ctx, server, "timeseries")
		if err != nil {
			log.Error(err, "Failed to ensure TimeSeries TLS certificate")
			return ctrl.Result{}, err
		}
	}

	// Ensure Search authentication secret and certificate (if component is enabled)
	if r.isComponentEnabled(server.Spec.Search) {
		var err error
		searchSecretName, err = r.ensureAuthenticationSecret(ctx, server, "search", server.Spec.Search.Authentication)
		if err != nil {
			log.Error(err, "Failed to ensure Search authentication secret")
			return ctrl.Result{}, err
		}

		// Create TLS certificate for Search
		searchCertSecretName, err = r.ensureTLSCertificate(ctx, server, "search")
		if err != nil {
			log.Error(err, "Failed to ensure Search TLS certificate")
			return ctrl.Result{}, err
		}
	}

	// Re-fetch CR before status update to avoid conflicts
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update status with secret names
	statusChanged := server.Status.TimeSeriesSecretName != timeSeriesSecretName ||
		server.Status.SearchSecretName != searchSecretName ||
		server.Status.TimeSeriesCertSecretName != timeSeriesCertSecretName ||
		server.Status.SearchCertSecretName != searchCertSecretName

	if statusChanged {
		server.Status.TimeSeriesSecretName = timeSeriesSecretName
		server.Status.SearchSecretName = searchSecretName
		server.Status.TimeSeriesCertSecretName = timeSeriesCertSecretName
		server.Status.SearchCertSecretName = searchCertSecretName
		server.Status.ObservedGeneration = server.Generation
		if err := r.Status().Update(ctx, server); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		log.Info("Updated status",
			"timeSeriesSecret", timeSeriesSecretName,
			"searchSecret", searchSecretName,
			"timeSeriesCertSecret", timeSeriesCertSecretName,
			"searchCertSecret", searchCertSecretName)
	}

	return ctrl.Result{}, nil
}

// ensureAuthenticationSecret ensures a Secret exists for the component's authentication.
// Returns the name of the secret being used.
//
// Priority:
// 1. If SecretRef is set, verify it exists and return its name
// 2. If Username/Password are provided, create a Secret with those values
// 3. Otherwise, generate random credentials and create a Secret
func (r *AxonOpsServerReconciler) ensureAuthenticationSecret(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
	component string,
	auth corev1alpha1.AxonAuthentication,
) (string, error) {
	log := logf.FromContext(ctx)

	// Get the correct secret keys for this component
	userKey, passwordKey := r.getSecretKeys(component)

	// Case 1: User provided a SecretRef - verify it exists
	if auth.SecretRef != "" {
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      auth.SecretRef,
			Namespace: server.Namespace,
		}, secret)
		if err != nil {
			return "", fmt.Errorf("referenced secret %q not found: %w", auth.SecretRef, err)
		}
		// Verify required keys exist
		if _, ok := secret.Data[userKey]; !ok {
			return "", fmt.Errorf("secret %q missing required key %q", auth.SecretRef, userKey)
		}
		if _, ok := secret.Data[passwordKey]; !ok {
			return "", fmt.Errorf("secret %q missing required key %q", auth.SecretRef, passwordKey)
		}
		log.Info("Using existing secret for authentication", "component", component, "secret", auth.SecretRef)
		return auth.SecretRef, nil
	}

	// Case 2 & 3: Create or update a managed secret
	secretName := fmt.Sprintf("%s-%s-auth", server.Name, component)

	// Determine credentials
	username := auth.Username
	password := auth.Password

	// Check if secret already exists (to preserve generated credentials)
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: server.Namespace,
	}, existingSecret)

	if err == nil {
		// Secret exists - if no explicit credentials provided, keep existing ones
		if username == "" && password == "" {
			log.Info("Using existing generated secret", "component", component, "secret", secretName)
			return secretName, nil
		}
		// User provided credentials - update the secret
		if username == "" {
			username = string(existingSecret.Data[userKey])
		}
		if password == "" {
			password = string(existingSecret.Data[passwordKey])
		}
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing secret: %w", err)
	} else {
		// Secret doesn't exist - generate credentials if not provided
		if username == "" {
			username = fmt.Sprintf("%s_%s", defaultUsernamePrefix, component)
		}
		if password == "" {
			password, err = generateRandomPassword(32)
			if err != nil {
				return "", fmt.Errorf("failed to generate password: %w", err)
			}
			log.Info("Generated random credentials", "component", component, "username", username)
		}
	}

	// Create or update the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// Set owner reference for garbage collection
		if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
			return err
		}

		// Set labels
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		secret.Labels["app.kubernetes.io/name"] = "axonops"
		secret.Labels["app.kubernetes.io/component"] = component
		secret.Labels["app.kubernetes.io/managed-by"] = "axonops-operator"

		// Set secret data with component-specific keys
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			userKey:     []byte(username),
			passwordKey: []byte(password),
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to create/update secret: %w", err)
	}

	log.Info("Secret reconciled", "component", component, "secret", secretName, "operation", op)
	return secretName, nil
}

// getSecretKeys returns the appropriate secret key names for a given component.
func (r *AxonOpsServerReconciler) getSecretKeys(component string) (userKey, passwordKey string) {
	switch component {
	case "timeseries":
		return timeseriesSecretKeyUser, timeseriesSecretKeyPassword
	case "search":
		return searchSecretKeyUser, searchSecretKeyPassword
	default:
		// Default to timeseries keys for unknown components
		return timeseriesSecretKeyUser, timeseriesSecretKeyPassword
	}
}

// ensureTLSCertificate ensures a cert-manager Certificate exists for the component.
// Returns the name of the secret that will contain the TLS certificate.
func (r *AxonOpsServerReconciler) ensureTLSCertificate(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
	component string,
) (string, error) {
	log := logf.FromContext(ctx)

	certName := fmt.Sprintf("%s-%s-tls", server.Name, component)
	secretName := certName // cert-manager creates a secret with the same name as the certificate

	// Build the Certificate resource
	cert := &certmanagerv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, func() error {
		// Set owner reference for garbage collection
		if err := controllerutil.SetControllerReference(server, cert, r.Scheme); err != nil {
			return err
		}

		// Set labels
		if cert.Labels == nil {
			cert.Labels = make(map[string]string)
		}
		cert.Labels["app.kubernetes.io/name"] = "axonops"
		cert.Labels["app.kubernetes.io/component"] = component
		cert.Labels["app.kubernetes.io/managed-by"] = "axonops-operator"

		// Configure the certificate spec
		cert.Spec = certmanagerv1.CertificateSpec{
			SecretName: secretName,
			IssuerRef: cmmeta.IssuerReference{
				Name: r.ClusterIssuerName,
				Kind: "ClusterIssuer",
			},
			CommonName: fmt.Sprintf("%s-%s", server.Name, component),
			DNSNames: []string{
				// Service DNS names within the cluster
				fmt.Sprintf("%s-%s", server.Name, component),
				fmt.Sprintf("%s-%s.%s", server.Name, component, server.Namespace),
				fmt.Sprintf("%s-%s.%s.svc", server.Name, component, server.Namespace),
				fmt.Sprintf("%s-%s.%s.svc.cluster.local", server.Name, component, server.Namespace),
			},
			Duration: &metav1.Duration{
				Duration: 90 * 24 * time.Hour, // 90 days
			},
			RenewBefore: &metav1.Duration{
				Duration: 30 * 24 * time.Hour, // Renew 30 days before expiry
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Size:      2048,
			},
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageServerAuth,
				certmanagerv1.UsageClientAuth,
				certmanagerv1.UsageDigitalSignature,
				certmanagerv1.UsageKeyEncipherment,
			},
		}

		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to create/update certificate: %w", err)
	}

	log.Info("Certificate reconciled", "component", component, "certificate", certName, "operation", op)
	return secretName, nil
}

// isComponentEnabled checks if a component is enabled.
// A component is enabled if it's not nil and its Enabled field is true (default).
func (r *AxonOpsServerReconciler) isComponentEnabled(component any) bool {
	if component == nil {
		return false
	}
	switch c := component.(type) {
	case *corev1alpha1.AxonDbComponent:
		return c != nil && c.Enabled
	case *corev1alpha1.AxonServerComponent:
		return c != nil && c.Enabled
	case *corev1alpha1.AxonDashboardComponent:
		return c != nil && c.Enabled
	default:
		return false
	}
}

// generateRandomPassword generates a cryptographically secure random password.
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AxonOpsServer{}).
		Owns(&corev1.Secret{}).
		Owns(&certmanagerv1.Certificate{}).
		Named("axonopsserver").
		Complete(r)
}
