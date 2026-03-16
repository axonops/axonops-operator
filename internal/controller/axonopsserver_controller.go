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
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strings"
	"text/template"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
)

const (
	// Secret keys for database credentials
	searchSecretKeyUser     = "AXONOPS_SEARCH_USER"
	searchSecretKeyPassword = "AXONOPS_SEARCH_PASSWORD"

	timeseriesSecretKeyUser     = "AXONOPS_DB_USER"
	timeseriesSecretKeyPassword = "AXONOPS_DB_PASSWORD"
	// Default username prefix for auto-generated credentials
	defaultUsernamePrefix = "axonops"

	// Default images
	defaultTimeseriesImage = "ghcr.io/axonops/axondb-timeseries"
	defaultTimeseriesTag   = "5.0.6-1.1.0"
	defaultSearchImage     = "ghcr.io/axonops/axondb-search"
	defaultSearchTag       = "3.3.2-1.5.0"
	defaultServerImage     = "registry.axonops.com/axonops-public/axonops-docker/axon-server"
	defaultServerTag       = "2.0.27"
	defaultDashboardImage  = "registry.axonops.com/axonops-public/axonops-docker/axon-dash"
	defaultDashboardTag    = "2.0.28"

	// Default heap sizes
	defaultTimeseriesHeapSize = "1024M"
	defaultSearchHeapSize     = "2g"

	// Default storage sizes
	defaultStorageSize       = "10Gi"
	defaultServerStorageSize = "1Gi"

	// Server-specific constants
	serverUserID  = int64(9988)
	serverGroupID = int64(9988)

	// Component names
	componentTimeseries = "timeseries"
	componentSearch     = "search"
	componentServer     = "server"
	componentDashboard  = "dashboard"

	// Finalizer name for cleanup
	axonOpsServerFinalizer = "core.axonops.com/finalizer"
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
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete

// Reconcile moves the current state of the cluster closer to the desired state.
func (r *AxonOpsServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AxonOpsServer CR
	server := &corev1alpha1.AxonOpsServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Info("Reconciling AxonOpsServer", "name", req.NamespacedName)

	// Handle deletion
	if !server.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(server, axonOpsServerFinalizer) {
			// Clean up TLS secrets created by cert-manager (they don't get auto-deleted)
			if err := r.cleanupTLSSecrets(ctx, server); err != nil {
				log.Error(err, "Failed to cleanup TLS secrets")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(server, axonOpsServerFinalizer)
			if err := r.Update(ctx, server); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Removed finalizer and cleaned up resources")
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(server, axonOpsServerFinalizer) {
		controllerutil.AddFinalizer(server, axonOpsServerFinalizer)
		if err := r.Update(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Added finalizer")
		// Re-fetch after update
		if err := r.Get(ctx, req.NamespacedName, server); err != nil {
			return ctrl.Result{}, err
		}
	}

	var timeSeriesSecretName, searchSecretName string
	var timeSeriesCertSecretName, searchCertSecretName string

	// Ensure TimeSeries authentication secret, certificate, and workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.TimeSeries) {
		var err error
		timeSeriesSecretName, err = r.ensureAuthenticationSecret(ctx, server, componentTimeseries, server.Spec.TimeSeries.Authentication)
		if err != nil {
			log.Error(err, "Failed to ensure TimeSeries authentication secret")
			return ctrl.Result{}, err
		}

		// Create TLS certificate for TimeSeries
		timeSeriesCertSecretName, err = r.ensureTLSCertificate(ctx, server, componentTimeseries)
		if err != nil {
			log.Error(err, "Failed to ensure TimeSeries TLS certificate")
			return ctrl.Result{}, err
		}

		// Create ServiceAccount, Services, and StatefulSet for TimeSeries
		if err := r.reconcileTimeseries(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile TimeSeries workload")
			return ctrl.Result{}, err
		}
	}

	// Ensure Search authentication secret, certificate, and workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Search) {
		var err error
		searchSecretName, err = r.ensureAuthenticationSecret(ctx, server, componentSearch, server.Spec.Search.Authentication)
		if err != nil {
			log.Error(err, "Failed to ensure Search authentication secret")
			return ctrl.Result{}, err
		}

		// Create TLS certificate for Search
		searchCertSecretName, err = r.ensureTLSCertificate(ctx, server, componentSearch)
		if err != nil {
			log.Error(err, "Failed to ensure Search TLS certificate")
			return ctrl.Result{}, err
		}

		// Create ServiceAccount, Services, and StatefulSet for Search
		if err := r.reconcileSearch(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile Search workload")
			return ctrl.Result{}, err
		}
	}

	// Ensure Server workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Server) {
		// Create ServiceAccount, Services, Config Secret, and StatefulSet for Server
		if err := r.reconcileServer(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile Server workload")
			return ctrl.Result{}, err
		}
	}

	// Ensure Dashboard workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Dashboard) {
		// Create ServiceAccount, Service, ConfigMap, and Deployment for Dashboard
		if err := r.reconcileDashboard(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile Dashboard workload")
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

// cleanupTLSSecrets deletes TLS secrets created by cert-manager.
// These secrets don't get auto-deleted when the Certificate is deleted.
func (r *AxonOpsServerReconciler) cleanupTLSSecrets(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	// List of TLS secrets to delete (created by cert-manager for each component)
	tlsSecrets := []string{
		fmt.Sprintf("%s-%s-tls", server.Name, componentTimeseries),
		fmt.Sprintf("%s-%s-tls", server.Name, componentSearch),
	}

	for _, secretName := range tlsSecrets {
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: server.Namespace,
		}, secret)

		if err != nil {
			if errors.IsNotFound(err) {
				continue // Secret doesn't exist, skip
			}
			return fmt.Errorf("failed to get TLS secret %s: %w", secretName, err)
		}

		// Delete the secret
		if err := r.Delete(ctx, secret); err != nil {
			if errors.IsNotFound(err) {
				continue // Already deleted
			}
			return fmt.Errorf("failed to delete TLS secret %s: %w", secretName, err)
		}
		log.Info("Deleted TLS secret", "name", secretName)
	}

	return nil
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
			username = defaultUsernamePrefix
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

// ensureKeystorePasswordSecret ensures a Secret exists with a password for Java keystores.
// This is used by cert-manager to create JKS and PKCS12 keystores.
func (r *AxonOpsServerReconciler) ensureKeystorePasswordSecret(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
	component string,
) (string, error) {
	log := logf.FromContext(ctx)
	secretName := fmt.Sprintf("%s-%s-keystore-password", server.Name, component)

	// Check if secret already exists to preserve the password
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: server.Namespace,
	}, existingSecret)

	var password string
	if err == nil {
		// Secret exists, preserve existing password
		if p, ok := existingSecret.Data["password"]; ok {
			password = string(p)
		}
	} else if !errors.IsNotFound(err) {
		return "", fmt.Errorf("failed to check existing keystore password secret: %w", err)
	}

	// Generate password if not already set
	if password == "" {
		password, err = generateRandomPassword(24)
		if err != nil {
			return "", fmt.Errorf("failed to generate keystore password: %w", err)
		}
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
			return err
		}

		secret.Labels = r.buildLabels(server, component)
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			"password": []byte(password),
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to create/update keystore password secret: %w", err)
	}

	log.Info("Keystore password secret reconciled", "name", secretName, "operation", op)
	return secretName, nil
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

	// For search and timeseries components, ensure keystore password secret exists first
	// Both use Java keystores for TLS
	keystorePasswordSecretName := ""
	if component == componentSearch || component == componentTimeseries {
		var err error
		keystorePasswordSecretName, err = r.ensureKeystorePasswordSecret(ctx, server, component)
		if err != nil {
			return "", fmt.Errorf("failed to ensure keystore password secret: %w", err)
		}
	}

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

		// Build DNS names - include wildcard for StatefulSet pods
		serviceName := fmt.Sprintf("%s-%s", server.Name, component)
		headlessServiceName := fmt.Sprintf("%s-%s-headless", server.Name, component)
		dnsNames := []string{
			serviceName,
			fmt.Sprintf("%s.%s", serviceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc", serviceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, server.Namespace),
			fmt.Sprintf("*.%s.%s.svc.cluster.local", serviceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", headlessServiceName, server.Namespace),
		}

		// Configure the certificate spec
		cert.Spec = certmanagerv1.CertificateSpec{
			SecretName: secretName,
			IssuerRef: cmmeta.IssuerReference{
				Name: r.ClusterIssuerName,
				Kind: "ClusterIssuer",
			},
			CommonName: fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, server.Namespace),
			DNSNames:   dnsNames,
			Duration: &metav1.Duration{
				Duration: 90 * 24 * time.Hour, // 90 days
			},
			RenewBefore: &metav1.Duration{
				Duration: 30 * 24 * time.Hour, // Renew 30 days before expiry
			},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm: certmanagerv1.RSAKeyAlgorithm,
				Encoding:  certmanagerv1.PKCS1,
				Size:      2048,
			},
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageServerAuth,
				certmanagerv1.UsageClientAuth,
				certmanagerv1.UsageDigitalSignature,
				certmanagerv1.UsageKeyEncipherment,
			},
		}

		// For search and timeseries components, add JKS and PKCS12 keystores
		if (component == componentSearch || component == componentTimeseries) && keystorePasswordSecretName != "" {
			cert.Spec.Keystores = &certmanagerv1.CertificateKeystores{
				JKS: &certmanagerv1.JKSKeystore{
					Create: true,
					PasswordSecretRef: cmmeta.SecretKeySelector{
						LocalObjectReference: cmmeta.LocalObjectReference{
							Name: keystorePasswordSecretName,
						},
						Key: "password",
					},
				},
				PKCS12: &certmanagerv1.PKCS12Keystore{
					Create: true,
					PasswordSecretRef: cmmeta.SecretKeySelector{
						LocalObjectReference: cmmeta.LocalObjectReference{
							Name: keystorePasswordSecretName,
						},
						Key: "password",
					},
				},
			}
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

// generateRandomPassword generates a cryptographically secure random password
// that meets complexity requirements: min 8 chars, at least one uppercase,
// one lowercase, one digit, and one special character.
func generateRandomPassword(length int) (string, error) {
	const (
		lowercase = "abcdefghijklmnopqrstuvwxyz"
		uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digits    = "0123456789"
		special   = "!@#$%^&*"
		allChars  = lowercase + uppercase + digits + special
	)

	// Ensure minimum length of 8 to accommodate all required character types
	if length < 8 {
		length = 8
	}

	result := make([]byte, length)

	// Helper to pick a random character from a charset
	pickRandom := func(charset string) (byte, error) {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return 0, err
		}
		return charset[num.Int64()], nil
	}

	// Ensure at least one character from each required category
	var err error
	result[0], err = pickRandom(lowercase)
	if err != nil {
		return "", err
	}
	result[1], err = pickRandom(uppercase)
	if err != nil {
		return "", err
	}
	result[2], err = pickRandom(digits)
	if err != nil {
		return "", err
	}
	result[3], err = pickRandom(special)
	if err != nil {
		return "", err
	}

	// Fill the rest with random characters from all charsets
	for i := 4; i < length; i++ {
		result[i], err = pickRandom(allChars)
		if err != nil {
			return "", err
		}
	}

	// Shuffle the result to avoid predictable pattern
	for i := length - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		j := int(jBig.Int64())
		result[i], result[j] = result[j], result[i]
	}

	return string(result), nil
}

// reconcileTimeseries ensures all TimeSeries resources are created/updated
func (r *AxonOpsServerReconciler) reconcileTimeseries(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	// Ensure ServiceAccount
	if err := r.ensureServiceAccount(ctx, server, componentTimeseries); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount: %w", err)
	}

	// Ensure headless Service (for StatefulSet DNS)
	if err := r.ensureHeadlessService(ctx, server, componentTimeseries); err != nil {
		return fmt.Errorf("failed to ensure headless Service: %w", err)
	}

	// Ensure ClusterIP Service
	if err := r.ensureService(ctx, server, componentTimeseries); err != nil {
		return fmt.Errorf("failed to ensure Service: %w", err)
	}

	// Ensure StatefulSet
	if err := r.ensureTimeseriesStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet: %w", err)
	}

	log.Info("TimeSeries workload reconciled successfully")
	return nil
}

// ensureServiceAccount ensures a ServiceAccount exists for the component
func (r *AxonOpsServerReconciler) ensureServiceAccount(ctx context.Context, server *corev1alpha1.AxonOpsServer, component string) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, component)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
		if err := controllerutil.SetControllerReference(server, sa, r.Scheme); err != nil {
			return err
		}

		sa.Labels = r.buildLabels(server, component)
		sa.AutomountServiceAccountToken = ptr(true)
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("ServiceAccount reconciled", "name", name, "operation", op)
	return nil
}

// ensureHeadlessService ensures a headless Service exists for the StatefulSet
func (r *AxonOpsServerReconciler) ensureHeadlessService(ctx context.Context, server *corev1alpha1.AxonOpsServer, component string) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s-headless", server.Name, component)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
			return err
		}

		svc.Labels = r.buildLabels(server, component)
		svc.Annotations = map[string]string{
			"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
		}

		svc.Spec = corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector:                 r.buildSelectorLabels(server, component),
			Ports:                    r.getServicePorts(component, true),
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Headless Service reconciled", "name", name, "operation", op)
	return nil
}

// ensureService ensures a ClusterIP Service exists for the component
func (r *AxonOpsServerReconciler) ensureService(ctx context.Context, server *corev1alpha1.AxonOpsServer, component string) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, component)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
			return err
		}

		svc.Labels = r.buildLabels(server, component)
		svc.Spec = corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.buildSelectorLabels(server, component),
			Ports:    r.getServicePorts(component, false),
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Service reconciled", "name", name, "operation", op)
	return nil
}

// getServicePorts returns the appropriate service ports for a component
func (r *AxonOpsServerReconciler) getServicePorts(component string, headless bool) []corev1.ServicePort {
	switch component {
	case componentSearch:
		return []corev1.ServicePort{
			{Name: "http", Port: 9200, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
			{Name: "transport", Port: 9300, TargetPort: intstr.FromString("transport"), Protocol: corev1.ProtocolTCP},
			{Name: "metrics", Port: 9600, TargetPort: intstr.FromString("metrics"), Protocol: corev1.ProtocolTCP},
		}
	case componentTimeseries:
		if headless {
			// Headless service only needs the CQL port for DNS resolution
			return []corev1.ServicePort{
				{Name: "cql", Port: 9042, TargetPort: intstr.FromString("cql"), Protocol: corev1.ProtocolTCP},
			}
		}
		return []corev1.ServicePort{
			{Name: "cql", Port: 9042, TargetPort: intstr.FromString("cql"), Protocol: corev1.ProtocolTCP},
			{Name: "jmx", Port: 7199, TargetPort: intstr.FromString("jmx"), Protocol: corev1.ProtocolTCP},
		}
	case componentServer:
		// Server has both ports in its headless service
		return []corev1.ServicePort{
			{Name: "api", Port: 8080, TargetPort: intstr.FromString("api"), Protocol: corev1.ProtocolTCP},
			{Name: "agent", Port: 1888, TargetPort: intstr.FromString("agent"), Protocol: corev1.ProtocolTCP},
		}
	case componentDashboard:
		return []corev1.ServicePort{
			{Name: "http", Port: 3000, TargetPort: intstr.FromString("http"), Protocol: corev1.ProtocolTCP},
		}
	default:
		return nil
	}
}

// ensureTimeseriesStatefulSet ensures the TimeSeries StatefulSet exists
func (r *AxonOpsServerReconciler) ensureTimeseriesStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentTimeseries)
	headlessSvcName := fmt.Sprintf("%s-%s-headless", server.Name, componentTimeseries)

	// TLS secret names (created by ensureTLSCertificate and ensureKeystorePasswordSecret)
	tlsCertSecretName := fmt.Sprintf("%s-%s-tls", server.Name, componentTimeseries)
	keystorePasswordSecretName := fmt.Sprintf("%s-%s-keystore-password", server.Name, componentTimeseries)
	timeseriesAuthSecretName := fmt.Sprintf("%s-%s-auth", server.Name, componentTimeseries)

	// Get component config
	ts := server.Spec.TimeSeries

	// Determine image
	image := defaultTimeseriesImage
	tag := defaultTimeseriesTag
	pullPolicy := corev1.PullIfNotPresent
	if ts.Repository.Image != "" {
		image = ts.Repository.Image
	}
	if ts.Repository.Tag != "" {
		tag = ts.Repository.Tag
	}
	if ts.Repository.PullPolicy != "" {
		pullPolicy = ts.Repository.PullPolicy
	}

	// Determine heap size
	heapSize := defaultTimeseriesHeapSize
	if ts.HeapSize != "" {
		heapSize = ts.HeapSize
	}

	// Determine storage size
	storageSize := defaultStorageSize
	if ts.StorageConfig.Resources.Requests != nil {
		if storage, ok := ts.StorageConfig.Resources.Requests[corev1.ResourceStorage]; ok {
			storageSize = storage.String()
		}
	}

	// Build FQDN for Cassandra
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", name, server.Namespace)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}
	orgName := strings.ToLower(server.Spec.Server.OrgName)

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentTimeseries)
		selectorLabels := r.buildSelectorLabels(server, componentTimeseries)

		sts.Labels = labels
		sts.Spec = appsv1.StatefulSetSpec{
			Replicas:            ptr(int32(1)),
			ServiceName:         headlessSvcName,
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: ts.Annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup: ptr(int64(999)),
					},
					Containers: []corev1.Container{
						{
							Name:            "axondb-timeseries",
							Image:           fmt.Sprintf("%s:%s", image, tag),
							ImagePullPolicy: pullPolicy,
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: ptr(false),
								RunAsNonRoot:           ptr(true),
								RunAsUser:              ptr(int64(999)),
							},
							Env: r.buildTimeseriesEnv(fqdn, orgName, heapSize, keystorePasswordSecretName, timeseriesAuthSecretName, ts.Env),
							Ports: []corev1.ContainerPort{
								{Name: "cql", ContainerPort: 9042, Protocol: corev1.ProtocolTCP},
								{Name: "jmx", ContainerPort: 7199, Protocol: corev1.ProtocolTCP},
								{Name: "intra-node", ContainerPort: 7000, Protocol: corev1.ProtocolTCP},
								{Name: "intra-node-tls", ContainerPort: 7001, Protocol: corev1.ProtocolTCP},
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/bash", "-ec", "/usr/local/bin/healthcheck.sh liveness"},
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       30,
								TimeoutSeconds:      30,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/bash", "-ec", "/usr/local/bin/healthcheck.sh readiness"},
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       10,
								TimeoutSeconds:      30,
								SuccessThreshold:    1,
								FailureThreshold:    5,
							},
							Resources: ts.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/cassandra",
								},
								{
									Name:      "tls-certs",
									MountPath: "/etc/cassandra/certs",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: append([]corev1.Volume{
						{
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: tlsCertSecretName,
									Items: []corev1.KeyToPath{
										{Key: "keystore.jks", Path: "keystore.jks"},
										{Key: "truststore.jks", Path: "truststore.jks"},
										{Key: "keystore.p12", Path: "keystore.p12"},
										{Key: "truststore.p12", Path: "truststore.p12"},
										{Key: "tls.crt", Path: "tls.crt"},
										{Key: "tls.key", Path: "tls.key"},
										{Key: "ca.crt", Path: "ca.crt"},
									},
								},
							},
						},
					}, ts.ExtraVolumes...),
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "data",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
					},
				},
			},
		}

		// Add extra volume mounts if specified
		if len(ts.ExtraVolumeMounts) > 0 {
			sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				sts.Spec.Template.Spec.Containers[0].VolumeMounts,
				ts.ExtraVolumeMounts...,
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("StatefulSet reconciled", "name", name, "operation", op)
	return nil
}

// buildTimeseriesEnv builds environment variables for the timeseries container
func (r *AxonOpsServerReconciler) buildTimeseriesEnv(fqdn, orgName, heapSize, keystorePasswordSecretName, authSecretName string, extraEnv []corev1.EnvVar) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"},
			},
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
			},
		},
		{Name: "AXON_AGENT_ORG", Value: orgName},
		{Name: "CASSANDRA_FQDN", Value: fqdn},
		{Name: "CASSANDRA_HEAP_SIZE", Value: heapSize},
		// TLS/SSL Environment Variables
		{Name: "CASSANDRA_KEYSTORE_PATH", Value: "/etc/cassandra/certs/keystore.jks"},
		{Name: "CASSANDRA_TRUSTSTORE_PATH", Value: "/etc/cassandra/certs/truststore.jks"},
		{
			Name: "CASSANDRA_KEYSTORE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: keystorePasswordSecretName},
					Key:                  "password",
				},
			},
		},
		{
			Name: "CASSANDRA_TRUSTSTORE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: keystorePasswordSecretName},
					Key:                  "password",
				},
			},
		},
		// PEM certificate paths
		{Name: "CASSANDRA_TLS_KEY_PATH", Value: "/etc/cassandra/certs/tls.key"},
		{Name: "CASSANDRA_TLS_CERT_PATH", Value: "/etc/cassandra/certs/tls.crt"},
		{Name: "CASSANDRA_CA_CERT_PATH", Value: "/etc/cassandra/certs/ca.crt"},
		// Common TLS configuration
		{Name: "CASSANDRA_INTERNODE_ENCRYPTION", Value: "all"},
		{Name: "CASSANDRA_INTERNODE_CLIENT_AUTH", Value: "true"},
		{Name: "CASSANDRA_INTERNODE_PROTOCOL", Value: "TLS"},
		{Name: "CASSANDRA_CLIENT_ENCRYPTION_ENABLED", Value: "true"},
		{Name: "CASSANDRA_CLIENT_ENCRYPTION_OPTIONAL", Value: "true"},
		{Name: "CASSANDRA_CLIENT_PROTOCOL", Value: "TLS"},
		// Database credentials
		{
			Name: "AXONOPS_DB_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: authSecretName},
					Key:                  timeseriesSecretKeyUser,
				},
			},
		},
		{
			Name: "AXONOPS_DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: authSecretName},
					Key:                  timeseriesSecretKeyPassword,
				},
			},
		},
	}

	// Combine base env with extra env vars
	return slices.Concat(env, extraEnv)
}

// reconcileSearch ensures all Search resources are created/updated
func (r *AxonOpsServerReconciler) reconcileSearch(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	// Ensure ServiceAccount
	if err := r.ensureServiceAccount(ctx, server, componentSearch); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount: %w", err)
	}

	// Ensure headless Service (for StatefulSet DNS)
	if err := r.ensureHeadlessService(ctx, server, componentSearch); err != nil {
		return fmt.Errorf("failed to ensure headless Service: %w", err)
	}

	// Ensure ClusterIP Service
	if err := r.ensureService(ctx, server, componentSearch); err != nil {
		return fmt.Errorf("failed to ensure Service: %w", err)
	}

	// Ensure StatefulSet
	if err := r.ensureSearchStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet: %w", err)
	}

	log.Info("Search workload reconciled successfully")
	return nil
}

// ensureSearchStatefulSet ensures the Search StatefulSet exists
func (r *AxonOpsServerReconciler) ensureSearchStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentSearch)
	headlessSvcName := fmt.Sprintf("%s-%s-headless", server.Name, componentSearch)

	// TLS secret names (created by ensureTLSCertificate and ensureKeystorePasswordSecret)
	tlsCertSecretName := fmt.Sprintf("%s-%s-tls", server.Name, componentSearch)
	keystorePasswordSecretName := fmt.Sprintf("%s-%s-keystore-password", server.Name, componentSearch)
	searchAuthSecretName := fmt.Sprintf("%s-%s-auth", server.Name, componentSearch)

	// Get component config
	search := server.Spec.Search

	// Determine image
	image := defaultSearchImage
	tag := defaultSearchTag
	pullPolicy := corev1.PullIfNotPresent
	if search.Repository.Image != "" {
		image = search.Repository.Image
	}
	if search.Repository.Tag != "" {
		tag = search.Repository.Tag
	}
	if search.Repository.PullPolicy != "" {
		pullPolicy = search.Repository.PullPolicy
	}

	// Determine heap size
	heapSize := defaultSearchHeapSize
	if search.HeapSize != "" {
		heapSize = search.HeapSize
	}

	// Determine storage size
	storageSize := defaultStorageSize
	if search.StorageConfig.Resources.Requests != nil {
		if storage, ok := search.StorageConfig.Resources.Requests[corev1.ResourceStorage]; ok {
			storageSize = storage.String()
		}
	}

	// Build FQDN for OpenSearch
	fqdn := fmt.Sprintf("%s.%s.svc.cluster.local", name, server.Namespace)
	clusterName := fmt.Sprintf("%s-cluster", name)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentSearch)
		selectorLabels := r.buildSelectorLabels(server, componentSearch)

		sts.Labels = labels
		sts.Annotations = map[string]string{
			"majorVersion": "3",
		}

		sts.Spec = appsv1.StatefulSetSpec{
			Replicas:            ptr(int32(1)),
			ServiceName:         headlessSvcName,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: search.Annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            name,
					AutomountServiceAccountToken:  ptr(false),
					TerminationGracePeriodSeconds: ptr(int64(120)),
					SecurityContext: &corev1.PodSecurityContext{
						FSGroup:   ptr(int64(999)),
						RunAsUser: ptr(int64(999)),
					},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 1,
									PodAffinityTerm: corev1.PodAffinityTerm{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "app.kubernetes.io/instance",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{server.Name},
												},
												{
													Key:      "app.kubernetes.io/name",
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{"axonops"},
												},
											},
										},
									},
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:            "fsgroup-volume",
							Image:           "busybox:latest",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sh", "-c"},
							Args:            []string{"chown -R 999:999 /var/lib/opensearch"},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr(int64(0)),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/opensearch",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "axondb-search",
							Image:           fmt.Sprintf("%s:%s", image, tag),
							ImagePullPolicy: pullPolicy,
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot: ptr(true),
								RunAsUser:    ptr(int64(999)),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"IPC_LOCK"},
								},
							},
							Env: r.buildSearchEnv(fqdn, headlessSvcName, clusterName, heapSize, keystorePasswordSecretName, searchAuthSecretName, search.Env),
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 9200, Protocol: corev1.ProtocolTCP},
								{Name: "transport", ContainerPort: 9300, Protocol: corev1.ProtocolTCP},
								{Name: "metrics", ContainerPort: 9600, Protocol: corev1.ProtocolTCP},
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/bash", "-ec", "/usr/local/bin/healthcheck.sh startup"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      3,
								FailureThreshold:    30,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/bash", "-ec", "/usr/local/bin/healthcheck.sh readiness"},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       20,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    10,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"/bin/bash", "-ec", "/usr/local/bin/healthcheck.sh readiness"},
									},
								},
								PeriodSeconds:    5,
								TimeoutSeconds:   3,
								FailureThreshold: 3,
							},
							Resources: search.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/usr/share/opensearch/data",
								},
								{
									Name:      "tls-certs",
									MountPath: "/etc/opensearch/certs",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: append([]corev1.Volume{
						{
							Name: "tls-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: tlsCertSecretName,
									Items: []corev1.KeyToPath{
										{Key: "keystore.jks", Path: "keystore.jks"},
										{Key: "truststore.jks", Path: "truststore.jks"},
										{Key: "keystore.p12", Path: "keystore.p12"},
										{Key: "truststore.p12", Path: "truststore.p12"},
										{Key: "tls.crt", Path: "tls.crt"},
										{Key: "tls.key", Path: "tls.key"},
										{Key: "ca.crt", Path: "ca.crt"},
									},
								},
							},
						},
					}, search.ExtraVolumes...),
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "data",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
					},
				},
			},
		}

		// Add extra volume mounts if specified
		if len(search.ExtraVolumeMounts) > 0 {
			sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				sts.Spec.Template.Spec.Containers[0].VolumeMounts,
				search.ExtraVolumeMounts...,
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("StatefulSet reconciled", "name", name, "operation", op)
	return nil
}

// buildSearchEnv builds environment variables for the search container
func (r *AxonOpsServerReconciler) buildSearchEnv(fqdn, headlessSvc, clusterName, heapSize, keystorePasswordSecretName, searchAuthSecretName string, extraEnv []corev1.EnvVar) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "bootstrap.memory_lock", Value: "false"},
		{
			Name: "node.name",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"},
			},
		},
		{Name: "OPENSEARCH_FQDN", Value: fqdn},
		{Name: "discovery.seed_hosts", Value: headlessSvc},
		{Name: "cluster.name", Value: clusterName},
		{Name: "network.host", Value: "0.0.0.0"},
		{Name: "OPENSEARCH_HEAP_SIZE", Value: heapSize},
		{Name: "node.roles", Value: "master,ingest,data,remote_cluster_client,"},
		{Name: "OPENSEARCH_JAVA_OPTS", Value: ""},
		{Name: "discovery.type", Value: "single-node"},
		// TLS/SSL Environment Variables
		{Name: "GENERATE_CERTS_ON_STARTUP", Value: "false"},
		{Name: "OPENSEARCH_KEYSTORE_PATH", Value: "/etc/opensearch/certs/keystore.jks"},
		{Name: "OPENSEARCH_TRUSTSTORE_PATH", Value: "/etc/opensearch/certs/truststore.jks"},
		{
			Name: "OPENSEARCH_KEYSTORE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: keystorePasswordSecretName},
					Key:                  "password",
				},
			},
		},
		{
			Name: "OPENSEARCH_TRUSTSTORE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: keystorePasswordSecretName},
					Key:                  "password",
				},
			},
		},
		// PEM certificate paths
		{Name: "OPENSEARCH_TLS_KEY_PATH", Value: "/etc/opensearch/certs/tls.key"},
		{Name: "OPENSEARCH_TLS_CERT_PATH", Value: "/etc/opensearch/certs/tls.crt"},
		{Name: "OPENSEARCH_CA_CERT_PATH", Value: "/etc/opensearch/certs/ca.crt"},
		{
			Name: "AXONOPS_SEARCH_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: searchAuthSecretName},
					Key:                  "AXONOPS_SEARCH_PASSWORD",
				},
			},
		},
		{
			Name: "AXONOPS_SEARCH_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: searchAuthSecretName},
					Key:                  "AXONOPS_SEARCH_USER",
				},
			},
		},
	}

	// Combine base env with extra env vars
	return slices.Concat(env, extraEnv)
}

// reconcileServer ensures all Server resources are created/updated
func (r *AxonOpsServerReconciler) reconcileServer(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	// Ensure ServiceAccount
	if err := r.ensureServiceAccount(ctx, server, componentServer); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount: %w", err)
	}

	// Ensure headless Service (for StatefulSet DNS)
	if err := r.ensureHeadlessService(ctx, server, componentServer); err != nil {
		return fmt.Errorf("failed to ensure headless Service: %w", err)
	}

	// Ensure Agent Service (separate service for agent port)
	if err := r.ensureServerAgentService(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure Agent Service: %w", err)
	}

	// Ensure API Service (separate service for API port)
	if err := r.ensureServerApiService(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure API Service: %w", err)
	}

	// Ensure Config Secret
	if err := r.ensureServerConfigSecret(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure Config Secret: %w", err)
	}

	// Ensure StatefulSet
	if err := r.ensureServerStatefulSet(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet: %w", err)
	}

	// Ensure Agent Ingress if enabled
	if server.Spec.Server.AgentIngress.Enabled {
		if err := r.ensureServerAgentIngress(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure Agent Ingress: %w", err)
		}
	}

	// Ensure API Ingress if enabled
	if server.Spec.Server.ApiIngress.Enabled {
		if err := r.ensureServerApiIngress(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure API Ingress: %w", err)
		}
	}

	// Ensure Agent Gateway if enabled
	if server.Spec.Server.AgentGateway.Enabled {
		if err := r.ensureServerAgentGateway(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure Agent Gateway: %w", err)
		}
	}

	// Ensure API Gateway if enabled
	if server.Spec.Server.ApiGateway.Enabled {
		if err := r.ensureServerApiGateway(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure API Gateway: %w", err)
		}
	}

	log.Info("Server workload reconciled successfully")
	return nil
}

// ensureServerAgentService ensures the Agent Service exists for the Server
func (r *AxonOpsServerReconciler) ensureServerAgentService(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s-agent", server.Name, componentServer)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentServer)
		labels["app.kubernetes.io/component"] = "agent"
		svc.Labels = labels

		svc.Spec = corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.buildSelectorLabels(server, componentServer),
			Ports: []corev1.ServicePort{
				{Name: "agent", Port: 1888, TargetPort: intstr.FromString("agent"), Protocol: corev1.ProtocolTCP},
			},
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Agent Service reconciled", "name", name, "operation", op)
	return nil
}

// ensureServerApiService ensures the API Service exists for the Server
func (r *AxonOpsServerReconciler) ensureServerApiService(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s-api", server.Name, componentServer)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentServer)
		labels["app.kubernetes.io/component"] = "api"
		svc.Labels = labels

		svc.Spec = corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: r.buildSelectorLabels(server, componentServer),
			Ports: []corev1.ServicePort{
				{Name: "api", Port: 8080, TargetPort: intstr.FromString("api"), Protocol: corev1.ProtocolTCP},
			},
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("API Service reconciled", "name", name, "operation", op)
	return nil
}

// ensureServerConfigSecret ensures the config Secret exists for the Server
func (r *AxonOpsServerReconciler) ensureServerConfigSecret(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentServer)

	// Build service URLs for search and timeseries
	searchURL := fmt.Sprintf("https://%s-%s:9200", server.Name, componentSearch)
	timeseriesHeadless := fmt.Sprintf("%s-%s-headless.%s.svc.cluster.local", server.Name, componentTimeseries, server.Namespace)

	// Build the axon-server.yml config (credentials are injected via env vars from secrets)
	configYAML := r.buildServerConfig(server, searchURL, timeseriesHeadless)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
			return err
		}

		secret.Labels = r.buildLabels(server, componentServer)
		secret.Type = corev1.SecretTypeOpaque
		secret.StringData = map[string]string{
			"axon-server.yml": configYAML,
		}
		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Config Secret reconciled", "name", name, "operation", op)
	return nil
}

// serverConfigData holds the data for the axon-server.yml template
type serverConfigData struct {
	SearchURL    string
	SearchCACert string
	OrgName      string
	DashURL      string
	CQLHosts     string
	CQLCACert    string
}

// serverConfigTemplate is the Go template for axon-server.yml
// Credentials are handled via environment variables from mounted secrets
const serverConfigTemplate = `agents_port: 1888
api_port: 8080
host: 0.0.0.0

search_db:
  hosts:
    - {{ .SearchURL }}
  skip_verify: true
  ca_cert: {{ .SearchCACert }}

org_name: {{ .OrgName }}

axon_dash_url: {{ .DashURL }}

# Log to stdout for Kubernetes
log_file: /dev/stdout
tls:
  mode: disabled
auth:
  enabled: false

retention:
  backups:
    local: 10d
    remote: 30d
  events: 8w
  metrics:
    high_resolution: 14d
    low_resolution: 12M
    med_resolution: 12w
    super_low_resolution: 2y
cql_autocreate_tables: true
cql_batch_size: 100
cql_hosts:
  - {{ .CQLHosts }}
cql_keyspace_replication: "{ 'class': 'NetworkTopologyStrategy', 'axonopsdb_dc1': 1 }"
cql_local_dc: axonopsdb_dc1
cql_max_searchqueriesparallelism: 100
cql_metrics_cache_max_items: 500000
cql_metrics_cache_max_size: 128
cql_page_size: 100
cql_proto_version: 4
cql_reconnectionpolicy_initialinterval: 1s
cql_reconnectionpolicy_maxinterval: 10s
cql_reconnectionpolicy_maxretries: 10
cql_retrypolicy_max: 10s
cql_retrypolicy_min: 2s
cql_retrypolicy_numretries: 3
cql_skip_verify: true
cql_ssl: true
cql_ca_cert: {{ .CQLCACert }}
`

// buildServerConfig generates the axon-server.yml configuration using a Go template.
// Credentials are passed via environment variables from mounted secrets.
func (r *AxonOpsServerReconciler) buildServerConfig(server *corev1alpha1.AxonOpsServer, searchURL, cqlHosts string) string {
	// Build dashboard URL from external hosts if available
	dashURL := fmt.Sprintf("https://%s-%s", server.Name, componentDashboard)
	if server.Spec.Dashboard != nil && len(server.Spec.Dashboard.External.Hosts) > 0 {
		dashURL = fmt.Sprintf("https://%s", server.Spec.Dashboard.External.Hosts[0])
	}

	data := serverConfigData{
		SearchURL:    searchURL,
		SearchCACert: "/etc/axonops/certs/search/ca.crt",
		OrgName:      strings.ToLower(server.Spec.Server.OrgName),
		DashURL:      dashURL,
		CQLHosts:     cqlHosts,
		CQLCACert:    "/etc/axonops/certs/timeseries/ca.crt",
	}

	tmpl, err := template.New("axon-server").Parse(serverConfigTemplate)
	if err != nil {
		// This should never happen with a valid template
		return fmt.Sprintf("# Template parse error: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Sprintf("# Template execute error: %v", err)
	}

	return buf.String()
}

// buildServerEnv builds environment variables for the server container
// including credentials from search and timeseries secrets
func (r *AxonOpsServerReconciler) buildServerEnv(server *corev1alpha1.AxonOpsServer, extraEnv []corev1.EnvVar) []corev1.EnvVar {
	// Build secret names based on the AxonOpsServer name
	searchSecretName := fmt.Sprintf("%s-%s-auth", server.Name, componentSearch)
	timeseriesSecretName := fmt.Sprintf("%s-%s-auth", server.Name, componentTimeseries)

	// Use status secret names if available (in case user provided their own secrets)
	if server.Status.SearchSecretName != "" {
		searchSecretName = server.Status.SearchSecretName
	}
	if server.Status.TimeSeriesSecretName != "" {
		timeseriesSecretName = server.Status.TimeSeriesSecretName
	}

	env := []corev1.EnvVar{
		// Search credentials (mapped to AXONOPS_SEARCH_USER/PASSWORD)
		{
			Name: "SEARCH_DB_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: searchSecretName},
					Key:                  searchSecretKeyUser,
					Optional:             ptr(true),
				},
			},
		},
		{
			Name: "SEARCH_DB_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: searchSecretName},
					Key:                  searchSecretKeyPassword,
					Optional:             ptr(true),
				},
			},
		},
		// TimeSeries credentials (mapped to AXONOPS_DB_USER/PASSWORD)
		{
			Name: "CQL_USERNAME",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: timeseriesSecretName},
					Key:                  timeseriesSecretKeyUser,
					Optional:             ptr(true),
				},
			},
		},
		{
			Name: "CQL_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: timeseriesSecretName},
					Key:                  timeseriesSecretKeyPassword,
					Optional:             ptr(true),
				},
			},
		},
	}

	// Combine base env with extra env vars from user config
	return slices.Concat(env, extraEnv)
}

// ensureServerStatefulSet ensures the Server StatefulSet exists
func (r *AxonOpsServerReconciler) ensureServerStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentServer)
	headlessSvcName := fmt.Sprintf("%s-%s-headless", server.Name, componentServer)
	configSecretName := name

	// TLS certificate secrets for backend connections
	searchTLSSecretName := fmt.Sprintf("%s-%s-tls", server.Name, componentSearch)
	timeseriesTLSSecretName := fmt.Sprintf("%s-%s-tls", server.Name, componentTimeseries)

	// Get component config
	srv := server.Spec.Server

	// Determine image
	image := defaultServerImage
	tag := defaultServerTag
	pullPolicy := corev1.PullIfNotPresent
	if srv.Repository.Image != "" {
		image = srv.Repository.Image
	}
	if srv.Repository.Tag != "" {
		tag = srv.Repository.Tag
	}
	if srv.Repository.PullPolicy != "" {
		pullPolicy = srv.Repository.PullPolicy
	}

	// Determine storage size
	storageSize := defaultServerStorageSize
	if srv.StorageConfig.Resources.Requests != nil {
		if storage, ok := srv.StorageConfig.Resources.Requests[corev1.ResourceStorage]; ok {
			storageSize = storage.String()
		}
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentServer)
		selectorLabels := r.buildSelectorLabels(server, componentServer)

		sts.Labels = labels
		sts.Spec = appsv1.StatefulSetSpec{
			Replicas:    ptr(int32(1)),
			ServiceName: headlessSvcName,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: srv.Annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					InitContainers: []corev1.Container{
						{
							Name:            "fsgroup-volume",
							Image:           "busybox:latest",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Command:         []string{"sh", "-c"},
							Args:            []string{fmt.Sprintf("chown -R %d:%d /var/lib/axonops", serverUserID, serverGroupID)},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser: ptr(int64(0)),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "data", MountPath: "/var/lib/axonops"},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "axon-server",
							Image:           fmt.Sprintf("%s:%s", image, tag),
							ImagePullPolicy: pullPolicy,
							Command:         []string{"/usr/share/axonops/axon-server", "-o", "stdout"},
							SecurityContext: &corev1.SecurityContext{
								ReadOnlyRootFilesystem: ptr(false),
								RunAsNonRoot:           ptr(true),
								RunAsUser:              ptr(serverUserID),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							Env: r.buildServerEnv(server, srv.Env),
							Ports: []corev1.ContainerPort{
								{Name: "api", ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
								{Name: "agent", ContainerPort: 1888, Protocol: corev1.ProtocolTCP},
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/v1/healthz",
										Port: intstr.FromString("api"),
									},
								},
								InitialDelaySeconds: 0,
								PeriodSeconds:       2,
								TimeoutSeconds:      3,
								FailureThreshold:    60,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/v1/healthz",
										Port: intstr.FromString("api"),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/api/v1/healthz",
										Port: intstr.FromString("api"),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							Resources: srv.Resources,
							VolumeMounts: []corev1.VolumeMount{
								{Name: "config", MountPath: "/etc/axonops"},
								{Name: "logs", MountPath: "/var/log/axonops"},
								{Name: "data", MountPath: "/var/lib/axonops"},
								{Name: "search-tls", MountPath: "/etc/axonops/certs/search", ReadOnly: true},
								{Name: "timeseries-tls", MountPath: "/etc/axonops/certs/timeseries", ReadOnly: true},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: configSecretName,
								},
							},
						},
						{
							Name: "logs",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "search-tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: searchTLSSecretName,
									Items: []corev1.KeyToPath{
										{Key: "ca.crt", Path: "ca.crt"},
										{Key: "tls.crt", Path: "tls.crt"},
										{Key: "tls.key", Path: "tls.key"},
									},
								},
							},
						},
						{
							Name: "timeseries-tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: timeseriesTLSSecretName,
									Items: []corev1.KeyToPath{
										{Key: "ca.crt", Path: "ca.crt"},
										{Key: "tls.crt", Path: "tls.crt"},
										{Key: "tls.key", Path: "tls.key"},
									},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "data",
						Labels: labels,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
					},
				},
			},
		}

		// Add extra volumes if specified
		if len(srv.ExtraVolumes) > 0 {
			sts.Spec.Template.Spec.Volumes = append(sts.Spec.Template.Spec.Volumes, srv.ExtraVolumes...)
		}

		// Add extra volume mounts if specified
		if len(srv.ExtraVolumeMounts) > 0 {
			sts.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				sts.Spec.Template.Spec.Containers[0].VolumeMounts,
				srv.ExtraVolumeMounts...,
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("StatefulSet reconciled", "name", name, "operation", op)
	return nil
}

// buildLabels builds standard labels for a component
func (r *AxonOpsServerReconciler) buildLabels(server *corev1alpha1.AxonOpsServer, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "axonops",
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/component":  component,
		"app.kubernetes.io/managed-by": "axonops-operator",
	}
}

// buildSelectorLabels builds selector labels for a component
func (r *AxonOpsServerReconciler) buildSelectorLabels(server *corev1alpha1.AxonOpsServer, component string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "axonops",
		"app.kubernetes.io/instance":  server.Name,
		"app.kubernetes.io/component": component,
	}
}

// reconcileDashboard ensures all Dashboard resources are created/updated
func (r *AxonOpsServerReconciler) reconcileDashboard(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	// Ensure ServiceAccount
	if err := r.ensureServiceAccount(ctx, server, componentDashboard); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount: %w", err)
	}

	// Ensure Service
	if err := r.ensureService(ctx, server, componentDashboard); err != nil {
		return fmt.Errorf("failed to ensure Service: %w", err)
	}

	// Ensure ConfigMap
	if err := r.ensureDashboardConfigMap(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure ConfigMap: %w", err)
	}

	// Ensure Deployment
	if err := r.ensureDashboardDeployment(ctx, server); err != nil {
		return fmt.Errorf("failed to ensure Deployment: %w", err)
	}

	// Ensure Ingress if enabled
	if server.Spec.Dashboard.Ingress.Enabled {
		if err := r.ensureDashboardIngress(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure Ingress: %w", err)
		}
	}

	// Ensure Gateway if enabled
	if server.Spec.Dashboard.Gateway.Enabled {
		if err := r.ensureDashboardGateway(ctx, server); err != nil {
			return fmt.Errorf("failed to ensure Gateway: %w", err)
		}
	}

	log.Info("Dashboard workload reconciled successfully")
	return nil
}

// ensureDashboardConfigMap ensures the Dashboard ConfigMap exists
func (r *AxonOpsServerReconciler) ensureDashboardConfigMap(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentDashboard)

	// Build the axon-dash.yml config
	serverAPIURL := fmt.Sprintf("http://%s-%s-api:8080", server.Name, componentServer)
	configYAML := fmt.Sprintf(`axon-server:
  private_endpoints: %s
  context_path: ""

axon-dash:
  host: 0.0.0.0
  port: 3000
  ssl:
    enabled: false
`, serverAPIURL)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := controllerutil.SetControllerReference(server, cm, r.Scheme); err != nil {
			return err
		}

		cm.Labels = r.buildLabels(server, componentDashboard)
		cm.Data = map[string]string{
			"axon-dash.yml": configYAML,
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("ConfigMap reconciled", "name", name, "operation", op)
	return nil
}

// ensureDashboardDeployment ensures the Dashboard Deployment exists
func (r *AxonOpsServerReconciler) ensureDashboardDeployment(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentDashboard)
	configMapName := name

	// Get component config
	dash := server.Spec.Dashboard

	// Determine image
	image := defaultDashboardImage
	tag := defaultDashboardTag
	pullPolicy := corev1.PullIfNotPresent
	if dash.Repository.Image != "" {
		image = dash.Repository.Image
	}
	if dash.Repository.Tag != "" {
		tag = dash.Repository.Tag
	}
	if dash.Repository.PullPolicy != "" {
		pullPolicy = dash.Repository.PullPolicy
	}

	// Determine replicas
	replicas := int32(1)
	if dash.Replicas != nil && *dash.Replicas > 0 {
		replicas = *dash.Replicas
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		if err := controllerutil.SetControllerReference(server, deploy, r.Scheme); err != nil {
			return err
		}

		labels := r.buildLabels(server, componentDashboard)
		selectorLabels := r.buildSelectorLabels(server, componentDashboard)

		deploy.Labels = labels
		deploy.Spec = appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: dash.Annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					Containers: []corev1.Container{
						{
							Name:            "axon-dash",
							Image:           fmt.Sprintf("%s:%s", image, tag),
							ImagePullPolicy: pullPolicy,
							Ports: []corev1.ContainerPort{
								{Name: "http", ContainerPort: 3000, Protocol: corev1.ProtocolTCP},
							},
							Env: dash.Env,
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/",
										Port: intstr.FromString("http"),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/axonops",
									ReadOnly:  true,
								},
							},
							Resources: dash.Resources,
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
								},
							},
						},
					},
				},
			},
		}

		// Add extra volumes if specified
		if len(dash.ExtraVolumes) > 0 {
			deploy.Spec.Template.Spec.Volumes = append(deploy.Spec.Template.Spec.Volumes, dash.ExtraVolumes...)
		}

		// Add extra volume mounts if specified
		if len(dash.ExtraVolumeMounts) > 0 {
			deploy.Spec.Template.Spec.Containers[0].VolumeMounts = append(
				deploy.Spec.Template.Spec.Containers[0].VolumeMounts,
				dash.ExtraVolumeMounts...,
			)
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Deployment reconciled", "name", name, "operation", op)
	return nil
}

// ensureIngress is a generic helper to create/update an Ingress resource
func (r *AxonOpsServerReconciler) ensureIngress(ctx context.Context, server *corev1alpha1.AxonOpsServer, ingressName, serviceName string, defaultPort int32, component string, ingressSpec corev1alpha1.Ingress) error {
	log := logf.FromContext(ctx)

	// Build ingress rules from hosts
	rules := make([]networkingv1.IngressRule, 0, len(ingressSpec.Hosts))
	for _, host := range ingressSpec.Hosts {
		pathType := networkingv1.PathTypePrefix
		if ingressSpec.PathType != "" {
			pathType = ingressSpec.PathType
		}
		path := "/"
		if ingressSpec.Path != "" {
			path = ingressSpec.Path
		}
		servicePort := defaultPort
		if ingressSpec.ServicePort > 0 {
			servicePort = ingressSpec.ServicePort
		}
		svcName := serviceName
		if ingressSpec.ServiceName != "" {
			svcName = ingressSpec.ServiceName
		}

		rules = append(rules, networkingv1.IngressRule{
			Host: host,
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{
							Path:     path,
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: svcName,
									Port: networkingv1.ServiceBackendPort{
										Number: servicePort,
									},
								},
							},
						},
					},
				},
			},
		})
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ingressName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, ingress, func() error {
		if err := controllerutil.SetControllerReference(server, ingress, r.Scheme); err != nil {
			return err
		}

		// Merge labels
		labels := r.buildLabels(server, component)
		maps.Copy(labels, ingressSpec.Labels)
		ingress.Labels = labels

		// Set annotations
		ingress.Annotations = ingressSpec.Annotations

		// Set IngressClassName if specified
		if ingressSpec.IngressClassName != "" {
			ingress.Spec.IngressClassName = &ingressSpec.IngressClassName
		}

		ingress.Spec.Rules = rules
		ingress.Spec.TLS = ingressSpec.TLS

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Ingress reconciled", "name", ingressName, "operation", op)
	return nil
}

// ensureDashboardIngress ensures the Dashboard Ingress exists
func (r *AxonOpsServerReconciler) ensureDashboardIngress(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	name := fmt.Sprintf("%s-%s", server.Name, componentDashboard)
	serviceName := name
	return r.ensureIngress(ctx, server, name, serviceName, 3000, componentDashboard, server.Spec.Dashboard.Ingress)
}

// ensureServerAgentIngress ensures the Server Agent Ingress exists
func (r *AxonOpsServerReconciler) ensureServerAgentIngress(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	name := fmt.Sprintf("%s-%s-agent", server.Name, componentServer)
	serviceName := name
	return r.ensureIngress(ctx, server, name, serviceName, 1888, componentServer, server.Spec.Server.AgentIngress)
}

// ensureServerApiIngress ensures the Server API Ingress exists
func (r *AxonOpsServerReconciler) ensureServerApiIngress(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	name := fmt.Sprintf("%s-%s-api", server.Name, componentServer)
	serviceName := name
	return r.ensureIngress(ctx, server, name, serviceName, 8080, componentServer, server.Spec.Server.ApiIngress)
}

// ensureHTTPRoute is a generic helper to create/update an HTTPRoute resource
func (r *AxonOpsServerReconciler) ensureHTTPRoute(ctx context.Context, server *corev1alpha1.AxonOpsServer, routeName, serviceName string, servicePort int32, component string, gatewaySpec corev1alpha1.GatewayConfig) error {
	log := logf.FromContext(ctx)

	// Use provided port or default
	port := gatewaySpec.Port
	if port == 0 {
		port = servicePort
	}

	httpRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      routeName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, httpRoute, func() error {
		if err := controllerutil.SetControllerReference(server, httpRoute, r.Scheme); err != nil {
			return err
		}

		// Merge labels
		labels := r.buildLabels(server, component)
		maps.Copy(labels, gatewaySpec.Labels)
		httpRoute.Labels = labels

		// Set annotations
		httpRoute.Annotations = gatewaySpec.Annotations

		// Build parent refs to the gateway
		gatewayName := fmt.Sprintf("%s-%s-gateway", server.Name, component)
		namespace := gatewayv1.Namespace(server.Namespace)
		httpRoute.Spec.ParentRefs = []gatewayv1.ParentReference{
			{
				Name:      gatewayv1.ObjectName(gatewayName),
				Namespace: &namespace,
			},
		}

		// Build hostname matching
		if gatewaySpec.Hostname != "" {
			hostname := gatewayv1.Hostname(gatewaySpec.Hostname)
			httpRoute.Spec.Hostnames = []gatewayv1.Hostname{hostname}
		}

		// Build backend refs
		portNum := gatewayv1.PortNumber(port) //nolint:unconvert
		httpRoute.Spec.Rules = []gatewayv1.HTTPRouteRule{
			{
				BackendRefs: []gatewayv1.HTTPBackendRef{
					{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: gatewayv1.ObjectName(serviceName),
								Port: &portNum,
							},
						},
					},
				},
			},
		}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("HTTPRoute reconciled", "name", routeName, "operation", op)
	return nil
}

// ensureGateway is a generic helper to create/update a Gateway resource
func (r *AxonOpsServerReconciler) ensureGateway(ctx context.Context, server *corev1alpha1.AxonOpsServer, gatewayName string, defaultPort int32, component string, gatewaySpec corev1alpha1.GatewayConfig) error {
	log := logf.FromContext(ctx)

	// Use provided port or default
	port := gatewaySpec.Port
	if port == 0 {
		port = defaultPort
	}

	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: server.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, gateway, func() error {
		if err := controllerutil.SetControllerReference(server, gateway, r.Scheme); err != nil {
			return err
		}

		// Merge labels
		labels := r.buildLabels(server, component)
		maps.Copy(labels, gatewaySpec.Labels)
		gateway.Labels = labels

		// Set annotations
		gateway.Annotations = gatewaySpec.Annotations

		// Set GatewayClassName
		gateway.Spec.GatewayClassName = gatewayv1.ObjectName(gatewaySpec.GatewayClassName)

		// Build listeners
		listenerPort := gatewayv1.PortNumber(port) //nolint:unconvert
		protocol := gatewayv1.HTTPProtocolType
		if port == 443 {
			protocol = gatewayv1.HTTPSProtocolType
		}

		listener := gatewayv1.Listener{
			Name:     gatewayv1.SectionName(component),
			Port:     listenerPort,
			Protocol: protocol,
		}

		// Add hostname if specified
		if gatewaySpec.Hostname != "" {
			hostname := gatewayv1.Hostname(gatewaySpec.Hostname)
			listener.Hostname = &hostname
		}

		gateway.Spec.Listeners = []gatewayv1.Listener{listener}

		return nil
	})

	if err != nil {
		return err
	}

	log.Info("Gateway reconciled", "name", gatewayName, "operation", op)
	return nil
}

// ensureDashboardGateway ensures the Dashboard Gateway and HTTPRoute exist
func (r *AxonOpsServerReconciler) ensureDashboardGateway(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	gatewayName := fmt.Sprintf("%s-%s-gateway", server.Name, componentDashboard)
	routeName := fmt.Sprintf("%s-%s-route", server.Name, componentDashboard)
	serviceName := fmt.Sprintf("%s-%s", server.Name, componentDashboard)

	// Create Gateway
	if err := r.ensureGateway(ctx, server, gatewayName, 3000, componentDashboard, server.Spec.Dashboard.Gateway); err != nil {
		return err
	}

	// Create HTTPRoute
	return r.ensureHTTPRoute(ctx, server, routeName, serviceName, 3000, componentDashboard, server.Spec.Dashboard.Gateway)
}

// ensureServerAgentGateway ensures the Server Agent Gateway and HTTPRoute exist
func (r *AxonOpsServerReconciler) ensureServerAgentGateway(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	gatewayName := fmt.Sprintf("%s-%s-agent-gateway", server.Name, componentServer)
	routeName := fmt.Sprintf("%s-%s-agent-route", server.Name, componentServer)
	serviceName := fmt.Sprintf("%s-%s-agent", server.Name, componentServer)

	// Create Gateway
	if err := r.ensureGateway(ctx, server, gatewayName, 1888, componentServer, server.Spec.Server.AgentGateway); err != nil {
		return err
	}

	// Create HTTPRoute
	return r.ensureHTTPRoute(ctx, server, routeName, serviceName, 1888, componentServer, server.Spec.Server.AgentGateway)
}

// ensureServerApiGateway ensures the Server API Gateway and HTTPRoute exist
func (r *AxonOpsServerReconciler) ensureServerApiGateway(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	gatewayName := fmt.Sprintf("%s-%s-api-gateway", server.Name, componentServer)
	routeName := fmt.Sprintf("%s-%s-api-route", server.Name, componentServer)
	serviceName := fmt.Sprintf("%s-%s-api", server.Name, componentServer)

	// Create Gateway
	if err := r.ensureGateway(ctx, server, gatewayName, 8080, componentServer, server.Spec.Server.ApiGateway); err != nil {
		return err
	}

	// Create HTTPRoute
	return r.ensureHTTPRoute(ctx, server, routeName, serviceName, 8080, componentServer, server.Spec.Server.ApiGateway)
}

// ptr returns a pointer to the given value
func ptr[T any](v T) *T {
	return &v
}

// SetupWithManager sets up the controller with the Manager.
func (r *AxonOpsServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.AxonOpsServer{}).
		Owns(&corev1.Secret{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&appsv1.Deployment{}).
		Owns(&networkingv1.Ingress{}).
		// Gateway API resources (Gateway, HTTPRoute) are owned but not watched.
		// They are created with owner references, enabling garbage collection.
		// We don't watch them because Gateway API CRDs may not be installed
		// in the cluster. The controller still creates/updates them when needed;
		// users configure Gateway support via spec.server.dashboard.external.gateway
		// and spec.server.agent.external.gateway fields in the AxonOpsServer CR.
		Owns(&certmanagerv1.Certificate{}).
		Named("axonopsserver").
		Complete(r)
}
