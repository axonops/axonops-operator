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
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"os"
	"slices"
	"strings"
	"text/template"
	"time"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	corev1alpha1 "github.com/axonops/axonops-operator/api/v1alpha1"
	axonopsmetrics "github.com/axonops/axonops-operator/internal/metrics"
	"sigs.k8s.io/yaml"
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

	// Default init container image (pinned version — do not use :latest)
	defaultInitImage = "docker.io/library/busybox:1.37.0"

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

	// App managed-by label value
	appManagedBy = "axonops-operator"
)

// AxonOpsServerReconciler reconciles a AxonOpsServer object
type AxonOpsServerReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	ClusterIssuerName string
	RESTMapper        meta.RESTMapper
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
// +kubebuilder:rbac:groups=cert-manager.io,resources=clusterissuers,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch

// resolveIssuerName returns the ClusterIssuer name to reference in Certificate
// resources. Priority: spec.tls.issuer.name > operator flag (ClusterIssuerName).
func (r *AxonOpsServerReconciler) resolveIssuerName(server *corev1alpha1.AxonOpsServer) string {
	if server.Spec.TLS.Issuer.Name != "" {
		return server.Spec.TLS.Issuer.Name
	}
	return r.ClusterIssuerName
}

// createOrUpdate wraps controllerutil.CreateOrUpdate and records Prometheus metrics.
func (r *AxonOpsServerReconciler) createOrUpdate(
	ctx context.Context, obj client.Object, f controllerutil.MutateFn, resourceType string,
) error {
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, obj, f)
	if err != nil {
		return err
	}
	switch op {
	case controllerutil.OperationResultCreated:
		axonopsmetrics.ResourceCreatedTotal.WithLabelValues(resourceType).Inc()
	case controllerutil.OperationResultUpdated:
		axonopsmetrics.ResourceUpdatedTotal.WithLabelValues(resourceType).Inc()
	}
	return nil
}

// needsInternalResources checks if the AxonOpsServer requires any internal database
// or internal workload components. Returns true if cert-manager is needed.
func needsInternalResources(server *corev1alpha1.AxonOpsServer) bool {
	// Check TimeSeries: internal if enabled and not external
	if server.Spec.TimeSeries != nil && (server.Spec.TimeSeries.Enabled == nil || *server.Spec.TimeSeries.Enabled) {
		if !isTimeSeriesExternal(server) {
			return true // Internal TimeSeries needs TLS certs
		}
	}
	// Check Search: internal if enabled and not external
	if server.Spec.Search != nil && (server.Spec.Search.Enabled == nil || *server.Spec.Search.Enabled) {
		if !isSearchExternal(server) {
			return true // Internal Search needs TLS certs
		}
	}
	return false
}

// isCertManagerAvailable checks if cert-manager Certificate CRD is registered
// in the cluster by querying the RESTMapper.
func (r *AxonOpsServerReconciler) isCertManagerAvailable() bool {
	_, err := r.RESTMapper.RESTMapping(
		schema.GroupKind{Group: "cert-manager.io", Kind: "Certificate"},
		"v1",
	)
	return err == nil
}

// ensureClusterIssuer creates or updates the default ClusterIssuer when no
// custom issuer name is provided via spec.tls.issuer.name.
// ClusterIssuer is cluster-scoped; no OwnerReference is set (intentional).
func (r *AxonOpsServerReconciler) ensureClusterIssuer(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
) error {
	log := logf.FromContext(ctx)

	issuerName := r.ClusterIssuerName
	clusterIssuer := &certmanagerv1.ClusterIssuer{
		ObjectMeta: metav1.ObjectMeta{
			Name: issuerName,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, clusterIssuer, func() error {
		if clusterIssuer.Labels == nil {
			clusterIssuer.Labels = make(map[string]string)
		}
		clusterIssuer.Labels["app.kubernetes.io/managed-by"] = appManagedBy

		if server.Spec.TLS.Issuer.CASecretRef != "" {
			// CA-backed issuer
			clusterIssuer.Spec = certmanagerv1.IssuerSpec{
				IssuerConfig: certmanagerv1.IssuerConfig{
					CA: &certmanagerv1.CAIssuer{
						SecretName: server.Spec.TLS.Issuer.CASecretRef,
					},
				},
			}
		} else {
			// SelfSigned issuer
			clusterIssuer.Spec = certmanagerv1.IssuerSpec{
				IssuerConfig: certmanagerv1.IssuerConfig{
					SelfSigned: &certmanagerv1.SelfSignedIssuer{},
				},
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update ClusterIssuer %q: %w", issuerName, err)
	}

	log.Info("ClusterIssuer reconciled", "name", issuerName, "operation", op)
	return nil
}

// Reconcile moves the current state of the cluster closer to the desired state.
// nolint:gocyclo
func (r *AxonOpsServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	log := logf.FromContext(ctx)

	// Start OpenTelemetry span for this reconciliation
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.axonopsserver",
		trace.WithAttributes())
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	// Record reconciliation duration and count metrics
	start := time.Now()
	defer func() {
		resultStr := axonopsmetrics.ResultSuccess
		if err != nil {
			resultStr = axonopsmetrics.ResultError
			axonopsmetrics.ReconcileErrorsTotal.WithLabelValues(axonopsmetrics.ClassifyError(err)).Inc()
		}
		axonopsmetrics.ReconcileDuration.WithLabelValues("axonopsserver", resultStr).Observe(time.Since(start).Seconds())
		axonopsmetrics.ReconcileTotal.WithLabelValues("axonopsserver", resultStr).Inc()
	}()

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

			// Clean up credential secrets that don't have owner references (Retain mode).
			// With Delete mode, owner references handle cascade deletion automatically.
			if err := r.cleanupCredentialSecretsOnDelete(ctx, server); err != nil {
				log.Error(err, "Failed to cleanup credential secrets")
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

	// Verify cert-manager CRDs are available (only needed for internal database/workload resources)
	if needsInternalResources(server) {
		if !r.isCertManagerAvailable() {
			// Re-fetch before status update
			if err := r.Get(ctx, req.NamespacedName, server); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
			meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
				Type:               "CertManagerReady",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: server.Generation,
				Reason:             "CertManagerUnavailable",
				Message:            "cert-manager CRDs are not installed in the cluster",
			})
			server.Status.ObservedGeneration = server.Generation
			if err := r.Status().Update(ctx, server); err != nil {
				log.Error(err, "Failed to update status for missing cert-manager CRDs")
			}
			log.Error(nil, "cert-manager CRDs not found; requeueing")
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}

		// Ensure default ClusterIssuer (only when no custom issuer name is specified)
		if server.Spec.TLS.Issuer.Name == "" {
			if err := r.ensureClusterIssuer(ctx, server); err != nil {
				// Re-fetch before status update
				if fetchErr := r.Get(ctx, req.NamespacedName, server); fetchErr != nil {
					return ctrl.Result{}, client.IgnoreNotFound(fetchErr)
				}
				meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
					Type:               "CertManagerReady",
					Status:             metav1.ConditionFalse,
					ObservedGeneration: server.Generation,
					Reason:             "FailedToCreateIssuer",
					Message:            err.Error(),
				})
				server.Status.ObservedGeneration = server.Generation
				if statusErr := r.Status().Update(ctx, server); statusErr != nil {
					log.Error(statusErr, "Failed to update status for ClusterIssuer creation failure")
				}
				log.Error(err, "Failed to ensure ClusterIssuer")
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "CertManagerReady",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: server.Generation,
			Reason:             "IssuerReady",
			Message:            "cert-manager is available and ClusterIssuer is ready",
		})
	}

	var timeSeriesSecretName, searchSecretName string
	var timeSeriesAuthHash, searchAuthHash string
	var timeSeriesCertSecretName, searchCertSecretName string

	// Ensure TimeSeries authentication secret, certificate, and workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.TimeSeries) {
		if isTimeSeriesExternal(server) {
			// Clean up any internal TimeSeries resources if they exist
			if err := r.cleanupInternalTimeseriesResources(ctx, server); err != nil {
				log.Error(err, "Failed to cleanup internal TimeSeries resources")
				return ctrl.Result{}, err
			}

			// Ensure authentication secret for external TimeSeries.
			// ensureAuthenticationSecret handles all cases: SecretRef (validates it),
			// inline username/password (creates a managed Secret), and no credentials
			// (auto-generates into a managed Secret).
			var err error
			timeSeriesSecretName, _, err = r.ensureAuthenticationSecret(ctx, server, componentTimeseries, server.Spec.TimeSeries.Authentication, server.Spec.TimeSeries.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure external TimeSeries authentication secret")
				return ctrl.Result{}, err
			}
			log.Info("TimeSeries is configured as external", "hosts", server.Spec.TimeSeries.External.Hosts)
		} else {
			// Internal timeseries - existing behavior
			var err error
			timeSeriesSecretName, timeSeriesAuthHash, err = r.ensureAuthenticationSecret(ctx, server, componentTimeseries, server.Spec.TimeSeries.Authentication, server.Spec.TimeSeries.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure TimeSeries authentication secret")
				return ctrl.Result{}, err
			}

			// Create TLS certificate for TimeSeries
			timeSeriesCertSecretName, err = r.ensureTLSCertificate(ctx, server, componentTimeseries, server.Spec.TimeSeries.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure TimeSeries TLS certificate")
				return ctrl.Result{}, err
			}

			// Create ServiceAccount, Services, and StatefulSet for TimeSeries
			if err := r.reconcileTimeseries(ctx, server, timeSeriesAuthHash); err != nil {
				log.Error(err, "Failed to reconcile TimeSeries workload")
				return ctrl.Result{}, err
			}
		}
	} else {
		// TimeSeries disabled — clean up any existing resources
		if err := r.cleanupInternalTimeseriesResources(ctx, server); err != nil {
			log.Error(err, "Failed to cleanup disabled TimeSeries resources")
			return ctrl.Result{}, err
		}
	}

	// Ensure Search authentication secret, certificate, and workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Search) {
		if isSearchExternal(server) {
			// Clean up any internal Search resources if they exist
			if err := r.cleanupInternalSearchResources(ctx, server); err != nil {
				log.Error(err, "Failed to cleanup internal Search resources")
				return ctrl.Result{}, err
			}

			// Ensure authentication secret for external Search.
			// ensureAuthenticationSecret handles all cases: SecretRef (validates it),
			// inline username/password (creates a managed Secret), and no credentials
			// (auto-generates into a managed Secret).
			var err error
			searchSecretName, _, err = r.ensureAuthenticationSecret(ctx, server, componentSearch, server.Spec.Search.Authentication, server.Spec.Search.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure external Search authentication secret")
				return ctrl.Result{}, err
			}
			log.Info("Search is configured as external", "hosts", server.Spec.Search.External.Hosts)
		} else {
			// Internal search - existing behavior
			var err error
			searchSecretName, searchAuthHash, err = r.ensureAuthenticationSecret(ctx, server, componentSearch, server.Spec.Search.Authentication, server.Spec.Search.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure Search authentication secret")
				return ctrl.Result{}, err
			}

			// Create TLS certificate for Search
			searchCertSecretName, err = r.ensureTLSCertificate(ctx, server, componentSearch, server.Spec.Search.StorageConfig)
			if err != nil {
				log.Error(err, "Failed to ensure Search TLS certificate")
				return ctrl.Result{}, err
			}

			// Create ServiceAccount, Services, and StatefulSet for Search
			if err := r.reconcileSearch(ctx, server, searchAuthHash); err != nil {
				log.Error(err, "Failed to reconcile Search workload")
				return ctrl.Result{}, err
			}
		}
	} else {
		// Search disabled — clean up any existing resources
		if err := r.cleanupInternalSearchResources(ctx, server); err != nil {
			log.Error(err, "Failed to cleanup disabled Search resources")
			return ctrl.Result{}, err
		}
	}

	// Gate: Server waits for TimeSeries and Search to be ready
	if r.isComponentEnabled(server.Spec.Server) {
		tsReady, tsReason := r.isComponentReady(ctx, server, componentTimeseries)
		searchReady, searchReason := r.isComponentReady(ctx, server, componentSearch)

		if !tsReady || !searchReady {
			var messages []string
			if !tsReady {
				messages = append(messages, tsReason)
			}
			if !searchReady {
				messages = append(messages, searchReason)
			}
			message := strings.Join(messages, "; ")

			meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
				Type:               "ServerReady",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: server.Generation,
				Reason:             "WaitingForDatabases",
				Message:            message,
			})
			if statusErr := r.Status().Update(ctx, server); statusErr != nil {
				log.Error(statusErr, "Failed to update status for database dependency wait")
			}
			log.Info("Server waiting for database dependencies", "timeseriesReady", tsReady, "searchReady", searchReady)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Track which components were successfully reconciled so conditions can be
	// applied after the re-fetch below (setting them here would be discarded).
	var serverReconciled, dashboardReconciled bool

	// Ensure Server workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Server) {
		// Create ServiceAccount, Services, Config Secret, and StatefulSet for Server
		if err := r.reconcileServer(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile Server workload")
			return ctrl.Result{}, err
		}
		serverReconciled = true
	} else {
		// Server disabled — clean up any existing resources
		if err := r.cleanupServerResources(ctx, server); err != nil {
			log.Error(err, "Failed to cleanup disabled Server resources")
			return ctrl.Result{}, err
		}
	}

	// Gate: Dashboard waits for Server to be ready
	if r.isComponentEnabled(server.Spec.Dashboard) {
		serverReady, serverReason := r.isComponentReady(ctx, server, componentServer)
		if !serverReady {
			meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
				Type:               "DashboardReady",
				Status:             metav1.ConditionFalse,
				ObservedGeneration: server.Generation,
				Reason:             "WaitingForServer",
				Message:            serverReason,
			})
			if statusErr := r.Status().Update(ctx, server); statusErr != nil {
				log.Error(statusErr, "Failed to update status for dashboard dependency wait")
			}
			log.Info("Dashboard waiting for Server to be ready")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Ensure Dashboard workload (if component is enabled)
	if r.isComponentEnabled(server.Spec.Dashboard) {
		// Create ServiceAccount, Service, ConfigMap, and Deployment for Dashboard
		if err := r.reconcileDashboard(ctx, server); err != nil {
			log.Error(err, "Failed to reconcile Dashboard workload")
			return ctrl.Result{}, err
		}
		dashboardReconciled = true
	} else {
		// Dashboard disabled — clean up any existing resources
		if err := r.cleanupDashboardResources(ctx, server); err != nil {
			log.Error(err, "Failed to cleanup disabled Dashboard resources")
			return ctrl.Result{}, err
		}
	}

	// Re-fetch CR before status update to avoid conflicts
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Update status with secret names and conditions
	statusChanged := server.Status.TimeSeriesSecretName != timeSeriesSecretName ||
		server.Status.SearchSecretName != searchSecretName ||
		server.Status.TimeSeriesCertSecretName != timeSeriesCertSecretName ||
		server.Status.SearchCertSecretName != searchCertSecretName

	// Set ServerReady and DashboardReady conditions — must be done after the
	// re-fetch above, otherwise SetStatusCondition changes are discarded.
	if serverReconciled {
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "ServerReady",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: server.Generation,
			Reason:             "Reconciled",
			Message:            "Server component reconciled successfully",
		})
		statusChanged = true
	}
	if dashboardReconciled {
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "DashboardReady",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: server.Generation,
			Reason:             "Reconciled",
			Message:            "Dashboard component reconciled successfully",
		})
		statusChanged = true
	}

	// Set TimeSeries mode condition
	if r.isComponentEnabled(server.Spec.TimeSeries) {
		timeseriesMode := "Internal"
		if isTimeSeriesExternal(server) {
			timeseriesMode = "External"
		}
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "TimeSeriesMode",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: server.Generation,
			Reason:             timeseriesMode,
			Message:            "TimeSeries: " + timeseriesMode,
		})
		statusChanged = true
	}

	// Set Search mode condition
	if r.isComponentEnabled(server.Spec.Search) {
		searchMode := "Internal"
		if isSearchExternal(server) {
			searchMode = "External"
		}
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "SearchMode",
			Status:             metav1.ConditionTrue,
			ObservedGeneration: server.Generation,
			Reason:             searchMode,
			Message:            "Search: " + searchMode,
		})
		statusChanged = true
	}

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

// cleanupCredentialSecretsOnDelete deletes credential secrets that should not be retained
// when the AxonOpsServer CR is being deleted. Secrets without owner references (Retain mode)
// won't be cascade-deleted, so we explicitly delete them only if retention is not wanted.
func (r *AxonOpsServerReconciler) cleanupCredentialSecretsOnDelete(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	type componentInfo struct {
		name          string
		storageConfig corev1.PersistentVolumeClaimSpec
	}

	var components []componentInfo
	if server.Spec.TimeSeries != nil {
		components = append(components, componentInfo{
			name:          componentTimeseries,
			storageConfig: server.Spec.TimeSeries.StorageConfig,
		})
	}
	if server.Spec.Search != nil {
		components = append(components, componentInfo{
			name:          componentSearch,
			storageConfig: server.Spec.Search.StorageConfig,
		})
	}

	for _, comp := range components {
		if r.shouldRetainCredentials(ctx, comp.storageConfig) {
			log.Info("Retaining credential secrets for reuse with existing PVCs", "component", comp.name)
			continue
		}

		// Delete auth and keystore secrets explicitly (they may lack owner refs if
		// the retention policy changed from Retain to Delete between reconciles)
		secretNames := []string{
			fmt.Sprintf("%s-%s-auth", server.Name, comp.name),
			fmt.Sprintf("%s-%s-keystore-password", server.Name, comp.name),
		}
		for _, name := range secretNames {
			secret := &corev1.Secret{}
			err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: server.Namespace}, secret)
			if errors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get secret %s: %w", name, err)
			}
			if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete secret %s: %w", name, err)
			}
			log.Info("Deleted credential secret", "name", name)
		}
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
// ensureAuthenticationSecret ensures an auth Secret exists for a database component.
// Returns (secretName, contentHash, error). The contentHash is used for rolling update
// annotations on the StatefulSet pod template.
func (r *AxonOpsServerReconciler) ensureAuthenticationSecret(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
	component string,
	auth corev1alpha1.AxonAuthentication,
	storageConfig corev1.PersistentVolumeClaimSpec,
) (string, string, error) {
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
			return "", "", fmt.Errorf("referenced secret %q not found: %w", auth.SecretRef, err)
		}
		// Verify required keys exist
		if _, ok := secret.Data[userKey]; !ok {
			return "", "", fmt.Errorf("secret %q missing required key %q", auth.SecretRef, userKey)
		}
		if _, ok := secret.Data[passwordKey]; !ok {
			return "", "", fmt.Errorf("secret %q missing required key %q", auth.SecretRef, passwordKey)
		}
		log.Info("Using existing secret for authentication", "component", component, "secret", auth.SecretRef)
		return auth.SecretRef, configDataHash(secret.Data), nil
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
			return secretName, configDataHash(existingSecret.Data), nil
		}
		// User provided credentials - update the secret
		if username == "" {
			username = string(existingSecret.Data[userKey])
		}
		if password == "" {
			password = string(existingSecret.Data[passwordKey])
		}
	} else if !errors.IsNotFound(err) {
		return "", "", fmt.Errorf("failed to check existing secret: %w", err)
	} else {
		// Secret doesn't exist - generate credentials if not provided
		if username == "" {
			username = defaultUsernamePrefix
		}
		if password == "" {
			password, err = generateRandomPassword(32)
			if err != nil {
				return "", "", fmt.Errorf("failed to generate password: %w", err)
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

	retain := r.shouldRetainCredentials(ctx, storageConfig)

	err = r.createOrUpdate(ctx, secret, func() error {
		if retain {
			// Remove owner reference so the secret survives CR deletion (PVCs are retained)
			r.removeOwnerReference(secret, server)
		} else {
			// Set owner reference for garbage collection (PVCs will be deleted)
			if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
				return err
			}
		}

		// Set labels for discovery on re-creation
		secret.Labels = r.buildLabels(server, component)

		// Set secret data with component-specific keys
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			userKey:     []byte(username),
			passwordKey: []byte(password),
		}

		return nil
	}, "secret")

	if err != nil {
		return "", "", fmt.Errorf("failed to create/update secret: %w", err)
	}

	// Compute hash from the credentials we just wrote (not from re-reading the Secret,
	// which could differ due to encoding). This ensures the hash is stable.
	contentHash := configDataHash(map[string][]byte{
		userKey:     []byte(username),
		passwordKey: []byte(password),
	})

	log.Info("Secret reconciled", "component", component, "secret", secretName)
	return secretName, contentHash, nil
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
	storageConfig corev1.PersistentVolumeClaimSpec,
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

	retain := r.shouldRetainCredentials(ctx, storageConfig)

	err = r.createOrUpdate(ctx, secret, func() error {
		if retain {
			r.removeOwnerReference(secret, server)
		} else {
			if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
				return err
			}
		}

		secret.Labels = r.buildLabels(server, component)
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = map[string][]byte{
			"password": []byte(password),
		}
		return nil
	}, "secret")

	if err != nil {
		return "", fmt.Errorf("failed to create/update keystore password secret: %w", err)
	}

	log.Info("Keystore password secret reconciled", "name", secretName, "retained", retain)
	return secretName, nil
}

// ensureTLSCertificate ensures a cert-manager Certificate exists for the component.
// Returns the name of the secret that will contain the TLS certificate.
func (r *AxonOpsServerReconciler) ensureTLSCertificate(
	ctx context.Context,
	server *corev1alpha1.AxonOpsServer,
	component string,
	storageConfig corev1.PersistentVolumeClaimSpec,
) (string, error) {
	log := logf.FromContext(ctx)

	certName := fmt.Sprintf("%s-%s-tls", server.Name, component)
	secretName := certName // cert-manager creates a secret with the same name as the certificate

	// For search and timeseries components, ensure keystore password secret exists first
	// Both use Java keystores for TLS
	keystorePasswordSecretName := ""
	if component == componentSearch || component == componentTimeseries {
		var err error
		keystorePasswordSecretName, err = r.ensureKeystorePasswordSecret(ctx, server, component, storageConfig)
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

	err := r.createOrUpdate(ctx, cert, func() error {
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
		cert.Labels["app.kubernetes.io/managed-by"] = appManagedBy

		// Build DNS names - include wildcard for StatefulSet pods
		serviceName := fmt.Sprintf("%s-%s", server.Name, component)
		headlessServiceName := fmt.Sprintf("%s-%s-headless", server.Name, component)
		dnsNames := []string{
			// ClusterIP service DNS names
			serviceName,
			fmt.Sprintf("%s.%s", serviceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc", serviceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, server.Namespace),
			// Headless service DNS names (all forms)
			headlessServiceName,
			fmt.Sprintf("%s.%s", headlessServiceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc", headlessServiceName, server.Namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", headlessServiceName, server.Namespace),
			// Wildcard for StatefulSet pod DNS: <pod>.<headless>.<ns>.svc.cluster.local
			fmt.Sprintf("*.%s.%s.svc.cluster.local", headlessServiceName, server.Namespace),
		}

		// Configure the certificate spec
		cert.Spec = certmanagerv1.CertificateSpec{
			SecretName: secretName,
			IssuerRef: cmmeta.IssuerReference{
				Name: r.resolveIssuerName(server),
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
	}, "certificate")

	if err != nil {
		return "", fmt.Errorf("failed to create/update certificate: %w", err)
	}

	log.Info("Certificate reconciled", "component", component, "certificate", certName)
	return secretName, nil
}

// cleanupInternalSearchResources deletes internal Search resources when switching to external search.
// Credential secrets are retained if the StorageClass reclaimPolicy is Retain.
func (r *AxonOpsServerReconciler) cleanupInternalSearchResources(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	var storageConfig corev1.PersistentVolumeClaimSpec
	if server.Spec.Search != nil {
		storageConfig = server.Spec.Search.StorageConfig
	}
	retain := r.shouldRetainCredentials(ctx, storageConfig)

	// Always delete workload resources
	resourcesToDelete := []struct {
		name string
		obj  client.Object
	}{
		{name: "Search StatefulSet", obj: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentSearch), Namespace: server.Namespace}}},
		{name: "Search headless Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-headless", server.Name, componentSearch), Namespace: server.Namespace}}},
		{name: "Search ClusterIP Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentSearch), Namespace: server.Namespace}}},
		{name: "Search TLS Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-tls", server.Name, componentSearch), Namespace: server.Namespace}}},
	}

	// Only delete credential secrets if storage is not retained
	if !retain {
		resourcesToDelete = append(resourcesToDelete,
			struct {
				name string
				obj  client.Object
			}{name: "Search auth Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s-auth", server.Name, componentSearch), Namespace: server.Namespace}}},
			struct {
				name string
				obj  client.Object
			}{name: "Search keystore Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s-keystore-password", server.Name, componentSearch), Namespace: server.Namespace}}},
		)
	} else {
		log.Info("Retaining Search credential secrets for reuse with existing PVCs")
	}

	for _, item := range resourcesToDelete {
		err := r.Delete(ctx, item.obj)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete internal resource", "type", item.name)
			return err
		}
		if err == nil {
			log.Info("Deleted internal Search resource", "type", item.name)
		}
	}

	return nil
}

// cleanupInternalTimeseriesResources deletes internal TimeSeries resources when switching to external timeseries.
// Credential secrets are retained if the StorageClass reclaimPolicy is Retain.
func (r *AxonOpsServerReconciler) cleanupInternalTimeseriesResources(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	var storageConfig corev1.PersistentVolumeClaimSpec
	if server.Spec.TimeSeries != nil {
		storageConfig = server.Spec.TimeSeries.StorageConfig
	}
	retain := r.shouldRetainCredentials(ctx, storageConfig)

	// Always delete workload resources
	resourcesToDelete := []struct {
		name string
		obj  client.Object
	}{
		{name: "TimeSeries StatefulSet", obj: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentTimeseries), Namespace: server.Namespace}}},
		{name: "TimeSeries headless Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-headless", server.Name, componentTimeseries), Namespace: server.Namespace}}},
		{name: "TimeSeries ClusterIP Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentTimeseries), Namespace: server.Namespace}}},
		{name: "TimeSeries TLS Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-tls", server.Name, componentTimeseries), Namespace: server.Namespace}}},
	}

	// Only delete credential secrets if storage is not retained
	if !retain {
		resourcesToDelete = append(resourcesToDelete,
			struct {
				name string
				obj  client.Object
			}{name: "TimeSeries auth Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s-auth", server.Name, componentTimeseries), Namespace: server.Namespace}}},
			struct {
				name string
				obj  client.Object
			}{name: "TimeSeries keystore Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-%s-keystore-password", server.Name, componentTimeseries), Namespace: server.Namespace}}},
		)
	} else {
		log.Info("Retaining TimeSeries credential secrets for reuse with existing PVCs")
	}

	for _, item := range resourcesToDelete {
		err := r.Delete(ctx, item.obj)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete internal resource", "type", item.name)
			return err
		}
		if err == nil {
			log.Info("Deleted internal TimeSeries resource", "type", item.name)
		}
	}

	return nil
}

// cleanupServerResources deletes all Server resources when the component is disabled
func (r *AxonOpsServerReconciler) cleanupServerResources(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	resourcesToDelete := []struct {
		name string
		obj  client.Object
	}{
		{name: "Server StatefulSet", obj: &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentServer), Namespace: server.Namespace}}},
		{name: "Server headless Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-headless", server.Name, componentServer), Namespace: server.Namespace}}},
		{name: "Server agent Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-agent", server.Name, componentServer), Namespace: server.Namespace}}},
		{name: "Server API Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-api", server.Name, componentServer), Namespace: server.Namespace}}},
		{name: "Server config Secret", obj: &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentServer), Namespace: server.Namespace}}},
		{name: "Server ServiceAccount", obj: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentServer), Namespace: server.Namespace}}},
	}

	for _, item := range resourcesToDelete {
		err := r.Delete(ctx, item.obj)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete Server resource", "type", item.name)
			return err
		}
		if err == nil {
			log.Info("Deleted Server resource", "type", item.name)
		}
	}

	return nil
}

// cleanupDashboardResources deletes all Dashboard resources when the component is disabled
func (r *AxonOpsServerReconciler) cleanupDashboardResources(ctx context.Context, server *corev1alpha1.AxonOpsServer) error {
	log := logf.FromContext(ctx)

	resourcesToDelete := []struct {
		name string
		obj  client.Object
	}{
		{name: "Dashboard Deployment", obj: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentDashboard), Namespace: server.Namespace}}},
		{name: "Dashboard Service", obj: &corev1.Service{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentDashboard), Namespace: server.Namespace}}},
		{name: "Dashboard ConfigMap", obj: &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentDashboard), Namespace: server.Namespace}}},
		{name: "Dashboard ServiceAccount", obj: &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s", server.Name, componentDashboard), Namespace: server.Namespace}}},
	}

	for _, item := range resourcesToDelete {
		err := r.Delete(ctx, item.obj)
		if err != nil && !errors.IsNotFound(err) {
			log.Error(err, "Failed to delete Dashboard resource", "type", item.name)
			return err
		}
		if err == nil {
			log.Info("Deleted Dashboard resource", "type", item.name)
		}
	}

	return nil
}

// isStatefulSetReady checks whether a StatefulSet has all its replicas ready.
// Returns false (not error) if the StatefulSet does not exist yet.
func (r *AxonOpsServerReconciler) isStatefulSetReady(ctx context.Context, namespace, name string) (bool, error) {
	sts := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, sts)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas == 0 {
		return false, nil
	}
	return sts.Status.ReadyReplicas == *sts.Spec.Replicas, nil
}

// isComponentReady checks whether a component's workload is ready.
// Disabled and external components are always considered ready.
// Internal components require their StatefulSet to have all replicas ready.
func (r *AxonOpsServerReconciler) isComponentReady(ctx context.Context, server *corev1alpha1.AxonOpsServer, component string) (bool, string) {
	switch component {
	case componentTimeseries:
		if !r.isComponentEnabled(server.Spec.TimeSeries) {
			return true, ""
		}
		if isTimeSeriesExternal(server) {
			return true, ""
		}
	case componentSearch:
		if !r.isComponentEnabled(server.Spec.Search) {
			return true, ""
		}
		if isSearchExternal(server) {
			return true, ""
		}
	case componentServer:
		if !r.isComponentEnabled(server.Spec.Server) {
			return true, ""
		}
	default:
		return true, ""
	}

	stsName := fmt.Sprintf("%s-%s", server.Name, component)
	ready, err := r.isStatefulSetReady(ctx, server.Namespace, stsName)
	if err != nil {
		return false, fmt.Sprintf("Failed to check %s readiness: %v", component, err)
	}
	if !ready {
		return false, fmt.Sprintf("Waiting for %s to become ready", component)
	}
	return true, ""
}

// A component is enabled if it's not nil and its Enabled field is true (default).
// A nil Enabled pointer is treated as true (enabled by default).
func (r *AxonOpsServerReconciler) isComponentEnabled(component any) bool {
	if component == nil {
		return false
	}
	switch c := component.(type) {
	case *corev1alpha1.AxonDbComponent:
		return c != nil && (c.Enabled == nil || *c.Enabled)
	case *corev1alpha1.AxonServerComponent:
		return c != nil && (c.Enabled == nil || *c.Enabled)
	case *corev1alpha1.AxonDashboardComponent:
		return c != nil && (c.Enabled == nil || *c.Enabled)
	default:
		return false
	}
}

// isSearchExternal checks if the Search component is configured to use external hosts
func isSearchExternal(server *corev1alpha1.AxonOpsServer) bool {
	return server.Spec.Search != nil && len(server.Spec.Search.External.Hosts) > 0
}

// isTimeSeriesExternal checks if the TimeSeries component is configured to use external hosts
func isTimeSeriesExternal(server *corev1alpha1.AxonOpsServer) bool {
	return server.Spec.TimeSeries != nil && len(server.Spec.TimeSeries.External.Hosts) > 0
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
func (r *AxonOpsServerReconciler) reconcileTimeseries(ctx context.Context, server *corev1alpha1.AxonOpsServer, authHash string) error {
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.timeseries")
	defer span.End()

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
	if err := r.ensureTimeseriesStatefulSet(ctx, server, authHash); err != nil {
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

	err := r.createOrUpdate(ctx, sa, func() error {
		if err := controllerutil.SetControllerReference(server, sa, r.Scheme); err != nil {
			return err
		}

		sa.Labels = r.buildLabels(server, component)
		sa.AutomountServiceAccountToken = ptr(true)
		return nil
	}, "serviceaccount")

	if err != nil {
		return err
	}

	log.Info("ServiceAccount reconciled", "name", name)
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

	err := r.createOrUpdate(ctx, svc, func() error {
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
	}, "service")

	if err != nil {
		return err
	}

	log.Info("Headless Service reconciled", "name", name)
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

	err := r.createOrUpdate(ctx, svc, func() error {
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
	}, "service")

	if err != nil {
		return err
	}

	log.Info("Service reconciled", "name", name)
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
func (r *AxonOpsServerReconciler) ensureTimeseriesStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer, authHash string) error {
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
	image := resolveImage(defaultTimeseriesImage, server.Spec.ImageRegistry, ts.Repository.Image)
	tag := defaultTimeseriesTag
	pullPolicy := corev1.PullIfNotPresent
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
	// Use OrgName from Server spec if available, otherwise fall back to CR name
	orgName := strings.ToLower(server.Name)
	if server.Spec.Server != nil && server.Spec.Server.OrgName != "" {
		orgName = strings.ToLower(server.Spec.Server.OrgName)
	}

	podAnnotations := mergeAnnotationsWithChecksum(ts.Annotations, "checksum/auth", authHash)

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
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					NodeSelector:       ts.NodeSelector,
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
func (r *AxonOpsServerReconciler) reconcileSearch(ctx context.Context, server *corev1alpha1.AxonOpsServer, authHash string) error {
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.search")
	defer span.End()

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
	if err := r.ensureSearchStatefulSet(ctx, server, authHash); err != nil {
		return fmt.Errorf("failed to ensure StatefulSet: %w", err)
	}

	log.Info("Search workload reconciled successfully")
	return nil
}

// ensureSearchStatefulSet ensures the Search StatefulSet exists
func (r *AxonOpsServerReconciler) ensureSearchStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer, authHash string) error {
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
	image := resolveImage(defaultSearchImage, server.Spec.ImageRegistry, search.Repository.Image)
	tag := defaultSearchTag
	pullPolicy := corev1.PullIfNotPresent
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

	podAnnotations := mergeAnnotationsWithChecksum(search.Annotations, "checksum/auth", authHash)

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
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            name,
					NodeSelector:                  search.NodeSelector,
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
							Image:           resolveInitImage(server),
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
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.server")
	defer span.End()

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

	// Ensure Config Secret (returns content hash for rolling update annotations)
	configHash, err := r.ensureServerConfigSecret(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to ensure Config Secret: %w", err)
	}

	// Ensure StatefulSet
	if err := r.ensureServerStatefulSet(ctx, server, configHash); err != nil {
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

// ensureServerConfigSecret ensures the config Secret exists for the Server.
// Returns the config content hash for use in pod template annotations.
func (r *AxonOpsServerReconciler) ensureServerConfigSecret(ctx context.Context, server *corev1alpha1.AxonOpsServer) (string, error) {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentServer)

	// Build service URLs for search and timeseries
	searchURL := fmt.Sprintf("https://%s-%s:9200", server.Name, componentSearch)
	if isSearchExternal(server) && len(server.Spec.Search.External.Hosts) > 0 {
		searchURL = server.Spec.Search.External.Hosts[0]
	}

	var cqlHosts string
	if isTimeSeriesExternal(server) && len(server.Spec.TimeSeries.External.Hosts) > 0 {
		cqlHosts = strings.Join(server.Spec.TimeSeries.External.Hosts, ",")
		dc := server.Spec.TimeSeries.External.DataCenter
		if dc == "" {
			dc = "(none)"
		}
		log.Info("Using external TimeSeries", "hosts", cqlHosts, "dataCenter", dc)
	} else {
		cqlHosts = fmt.Sprintf("%s-%s-headless.%s.svc.cluster.local", server.Name, componentTimeseries, server.Namespace)
	}

	// Resolve license key: inline key takes priority, then SecretRef
	var licenseKey string
	if server.Spec.Server.License.Key != "" {
		licenseKey = server.Spec.Server.License.Key
	} else if server.Spec.Server.License.SecretRef != "" {
		licenseSecret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      server.Spec.Server.License.SecretRef,
			Namespace: server.Namespace,
		}
		if err := r.Get(ctx, secretKey, licenseSecret); err != nil {
			return "", fmt.Errorf("failed to get license secret %q: %w", server.Spec.Server.License.SecretRef, err)
		}
		if key, ok := licenseSecret.Data["license_key"]; ok {
			licenseKey = string(key)
		}
	}

	// Build the axon-server.yml config (credentials are injected via env vars from secrets)
	configYAML, err := r.buildServerConfig(server, searchURL, cqlHosts, licenseKey)
	if err != nil {
		return "", fmt.Errorf("failed to build server config: %w", err)
	}

	// Compute hash from the generated config content (not from Secret.Data which may
	// differ due to StringData→Data encoding). This ensures the hash is stable across
	// operator restarts when the config content hasn't changed.
	contentHash := configStringDataHash(map[string]string{"axon-server.yml": configYAML})

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
		return "", err
	}

	log.Info("Config Secret reconciled", "name", name, "operation", op)
	return contentHash, nil
}

// serverConfigData holds the data for the axon-server.yml template
type serverConfigData struct {
	SearchURL              string
	SearchCACert           string
	SearchCert             string
	SearchKey              string
	SearchSkipVerify       bool
	OrgName                string
	LicenseKey             string
	DashURL                string
	CQLHosts               string
	CQLCACert              string
	CQLKey                 string
	CQLCert                string
	CQLSSLEnabled          bool
	CQLSkipVerify          bool
	CQLLocalDC             string
	CQLKeyspaceReplication string
}

// serverConfigTemplate is the Go template for axon-server.yml
// Credentials are handled via environment variables from mounted secrets
// FIXME: skip_verify should be set to false. It needs ticket ASB-4338 merged first
const serverConfigTemplate = `agents_port: 1888
api_port: 8080
host: 0.0.0.0
org_name: {{ .OrgName }}
{{ if .LicenseKey -}}
license_key: {{ .LicenseKey }}
{{ end -}}

search_db:
  hosts:
    - {{ .SearchURL }}
  skip_verify: {{ .SearchSkipVerify }}
  {{ if .SearchCACert -}}
  ca_file: {{ .SearchCACert }}
  {{ end -}}
  {{ if .SearchCert -}}
  cert_file: {{ .SearchCert }}
  {{ end -}}
  {{ if .SearchKey -}}
  key_file: {{ .SearchKey }}
  {{ end -}}


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
cql_keyspace_replication: "{{ .CQLKeyspaceReplication }}"
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
cql_skip_verify: {{ .CQLSkipVerify }}
cql_ssl: {{ .CQLSSLEnabled }}
{{ if .CQLCACert -}}
cql_ca_file: {{ .CQLCACert }}
{{ end -}}
{{ if .CQLKey -}}
cql_key_file: {{ .CQLKey }}
{{ end -}}
{{ if .CQLCert -}}
cql_cert_file: {{ .CQLCert }}
{{ end -}}
{{ if .CQLLocalDC }}
cql_local_dc: {{ .CQLLocalDC }}
{{ end -}}
`

// buildServerConfig generates the axon-server.yml configuration using a Go template.
// Credentials are passed via environment variables from mounted secrets.
func (r *AxonOpsServerReconciler) buildServerConfig(server *corev1alpha1.AxonOpsServer, searchURL, cqlHosts, licenseKey string) (string, error) {
	// Build dashboard URL from external hosts if available
	dashURL := fmt.Sprintf("https://%s-%s", server.Name, componentDashboard)
	if server.Spec.Dashboard != nil && len(server.Spec.Dashboard.External.Hosts) > 0 {
		dashURL = fmt.Sprintf("https://%s", server.Spec.Dashboard.External.Hosts[0])
	}

	// Determine search TLS settings based on internal vs external
	// Default to secure communication for internal connections
	searchSkipVerify := true
	searchCACert := "/etc/axonops/certs/search/ca.crt"
	searchKey := "/etc/axonops/certs/search/tls.key"
	searchCert := "/etc/axonops/certs/search/tls.crt"

	if isSearchExternal(server) && server.Spec.Search != nil {
		// For external search, respect the TLS configuration
		tls := server.Spec.Search.External.TLS
		if tls.Enabled {
			searchSkipVerify = tls.InsecureSkipVerify
			// Only include ca_cert if not skipping verification
			if !tls.InsecureSkipVerify {
				searchCACert = "/etc/axonops/certs/search/ca.crt"
			} else {
				searchCACert = ""
			}
		} else {
			// External search without TLS
			searchSkipVerify = false
			searchCACert = ""
		}
	}

	// Determine timeseries TLS settings based on internal vs external
	// Default to secure communication for internal connections
	cqlSSLEnabled := true
	cqlSkipVerify := true
	cqlCACert := "/etc/axonops/certs/timeseries/ca.crt"
	cqlKey := "/etc/axonops/certs/timeseries/tls.key"
	cqlCert := "/etc/axonops/certs/timeseries/tls.crt"
	localDC := "axonopsdb_dc1"

	if isTimeSeriesExternal(server) && server.Spec.TimeSeries != nil {
		dc := server.Spec.TimeSeries.External.DataCenter
		if dc != "" {
			localDC = dc
		} else {
			localDC = ""
		}
		// For external timeseries, respect the TLS configuration
		tls := server.Spec.TimeSeries.External.TLS
		if tls.Enabled {
			cqlSSLEnabled = true
			cqlSkipVerify = tls.InsecureSkipVerify
			// Only include cert paths if not skipping verification
			if !tls.InsecureSkipVerify {
				cqlCACert = "/etc/axonops/certs/timeseries/ca.crt"
				cqlKey = "/etc/axonops/certs/timeseries/tls.key"
				cqlCert = "/etc/axonops/certs/timeseries/tls.crt"
			} else {
				cqlCACert = ""
				cqlKey = ""
				cqlCert = ""
			}
		} else {
			// External timeseries without TLS
			cqlSSLEnabled = false
			cqlSkipVerify = false
			cqlCACert = ""
			cqlKey = ""
			cqlCert = ""
		}
	}

	cqlKeyspaceReplication := fmt.Sprintf("{ 'class': 'NetworkTopologyStrategy', '%s': 1 }", localDC)
	if localDC == "" {
		cqlKeyspaceReplication = "{ 'class': 'NetworkTopologyStrategy' }"
	}

	data := serverConfigData{
		SearchURL:              searchURL,
		SearchCACert:           searchCACert,
		SearchSkipVerify:       searchSkipVerify,
		SearchCert:             searchCert,
		SearchKey:              searchKey,
		OrgName:                strings.ToLower(server.Spec.Server.OrgName),
		LicenseKey:             licenseKey,
		DashURL:                dashURL,
		CQLHosts:               cqlHosts,
		CQLCACert:              cqlCACert,
		CQLKey:                 cqlKey,
		CQLCert:                cqlCert,
		CQLSSLEnabled:          cqlSSLEnabled,
		CQLSkipVerify:          cqlSkipVerify,
		CQLLocalDC:             localDC,
		CQLKeyspaceReplication: cqlKeyspaceReplication,
	}

	tmpl, err := template.New("axon-server").Parse(serverConfigTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse server config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute server config template: %w", err)
	}

	result := buf.String()

	// Append extra config if provided
	if server.Spec.Server.Config != nil && server.Spec.Server.Config.Raw != nil {
		var extraConfig map[string]any
		if err := json.Unmarshal(server.Spec.Server.Config.Raw, &extraConfig); err != nil {
			return "", fmt.Errorf("failed to unmarshal extra config: %w", err)
		}
		extraYAML, err := yaml.Marshal(extraConfig)
		if err != nil {
			return "", fmt.Errorf("failed to marshal extra config to YAML: %w", err)
		}
		result += "\n" + string(extraYAML)
	}

	return result, nil
}

// resolveSecretName returns the secret name for a component's authentication.
// Priority: spec SecretRef > status (from previous reconcile) > default convention.
func (r *AxonOpsServerReconciler) resolveSecretName(server *corev1alpha1.AxonOpsServer, component string) string {
	switch component {
	case componentTimeseries:
		if server.Spec.TimeSeries != nil && server.Spec.TimeSeries.Authentication.SecretRef != "" {
			return server.Spec.TimeSeries.Authentication.SecretRef
		}
		if server.Status.TimeSeriesSecretName != "" {
			return server.Status.TimeSeriesSecretName
		}
	case componentSearch:
		if server.Spec.Search != nil && server.Spec.Search.Authentication.SecretRef != "" {
			return server.Spec.Search.Authentication.SecretRef
		}
		if server.Status.SearchSecretName != "" {
			return server.Status.SearchSecretName
		}
	}
	return fmt.Sprintf("%s-%s-auth", server.Name, component)
}

// buildServerEnv builds environment variables for the server container
// including credentials from search and timeseries secrets
func (r *AxonOpsServerReconciler) buildServerEnv(server *corev1alpha1.AxonOpsServer, extraEnv []corev1.EnvVar) []corev1.EnvVar {
	searchSecretName := r.resolveSecretName(server, componentSearch)
	timeseriesSecretName := r.resolveSecretName(server, componentTimeseries)

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

// ensureServerStatefulSet ensures the Server StatefulSet exists.
// configHash is the pre-computed hash of the config Secret content.
func (r *AxonOpsServerReconciler) ensureServerStatefulSet(ctx context.Context, server *corev1alpha1.AxonOpsServer, configHash string) error {
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
	image := resolveImage(defaultServerImage, server.Spec.ImageRegistry, srv.Repository.Image)
	tag := defaultServerTag
	pullPolicy := corev1.PullIfNotPresent
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

	podAnnotations := mergeAnnotationsWithChecksum(srv.Annotations, "checksum/config", configHash)

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

		serverCmd := []string{"/usr/share/axonops/axon-server", "-o", "stdout"}
		if isDebug := os.Getenv("DEBUG"); isDebug == "true" {
			serverCmd = append(serverCmd, "-v", "1")
		}

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
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					NodeSelector:       srv.NodeSelector,
					InitContainers: []corev1.Container{
						{
							Name:            "fsgroup-volume",
							Image:           resolveInitImage(server),
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
							Command:         serverCmd,
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
							VolumeMounts: func() []corev1.VolumeMount {
								mounts := []corev1.VolumeMount{
									{Name: "config", MountPath: "/etc/axonops"},
									{Name: "logs", MountPath: "/var/log/axonops"},
									{Name: "data", MountPath: "/var/lib/axonops"},
								}
								// Only mount timeseries-tls if timeseries is internal
								if !isTimeSeriesExternal(server) {
									mounts = append(mounts, corev1.VolumeMount{Name: "timeseries-tls", MountPath: "/etc/axonops/certs/timeseries", ReadOnly: true})
								}
								// Only mount search-tls if search is internal
								if !isSearchExternal(server) {
									mounts = append(mounts, corev1.VolumeMount{Name: "search-tls", MountPath: "/etc/axonops/certs/search", ReadOnly: true})
								}
								return mounts
							}(),
						},
					},
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{
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
						}
						// Only include timeseries-tls volume if timeseries is internal
						if !isTimeSeriesExternal(server) {
							volumes = append(volumes, corev1.Volume{
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
							})
						}
						// Only include search-tls volume if search is internal
						if !isSearchExternal(server) {
							volumes = append(volumes, corev1.Volume{
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
							})
						}
						return volumes
					}(),
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
		"app.kubernetes.io/managed-by": appManagedBy,
	}
}

// removeOwnerReference removes the owner reference for the given owner from the object.
func (r *AxonOpsServerReconciler) removeOwnerReference(obj metav1.Object, owner metav1.Object) {
	ownerUID := owner.GetUID()
	refs := obj.GetOwnerReferences()
	filtered := make([]metav1.OwnerReference, 0, len(refs))
	for _, ref := range refs {
		if ref.UID != ownerUID {
			filtered = append(filtered, ref)
		}
	}
	obj.SetOwnerReferences(filtered)
}

// shouldRetainCredentials checks the StorageClass reclaimPolicy for a component's storage.
// Returns true if the StorageClass has Retain policy, meaning credential secrets should
// survive CR deletion so retained PVCs remain usable on re-creation.
func (r *AxonOpsServerReconciler) shouldRetainCredentials(ctx context.Context, storageConfig corev1.PersistentVolumeClaimSpec) bool {
	log := logf.FromContext(ctx)

	scName := ""
	if storageConfig.StorageClassName != nil {
		scName = *storageConfig.StorageClassName
	}

	// If no StorageClass specified, look up the cluster default
	if scName == "" {
		scList := &storagev1.StorageClassList{}
		if err := r.List(ctx, scList); err != nil {
			log.Error(err, "Could not list StorageClasses, defaulting to Delete behaviour")
			return false
		}
		for i := range scList.Items {
			if scList.Items[i].Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
				scName = scList.Items[i].Name
				break
			}
		}
		if scName == "" {
			return false
		}
	}

	sc := &storagev1.StorageClass{}
	if err := r.Get(ctx, client.ObjectKey{Name: scName}, sc); err != nil {
		log.Error(err, "Could not get StorageClass, defaulting to Delete behaviour", "storageClass", scName)
		return false
	}

	return sc.ReclaimPolicy != nil && *sc.ReclaimPolicy == corev1.PersistentVolumeReclaimRetain
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
	ctx, span := otel.Tracer("axonops-operator").Start(ctx, "reconcile.dashboard")
	defer span.End()

	log := logf.FromContext(ctx)

	// Ensure ServiceAccount
	if err := r.ensureServiceAccount(ctx, server, componentDashboard); err != nil {
		return fmt.Errorf("failed to ensure ServiceAccount: %w", err)
	}

	// Ensure Service
	if err := r.ensureService(ctx, server, componentDashboard); err != nil {
		return fmt.Errorf("failed to ensure Service: %w", err)
	}

	// Ensure ConfigMap (returns content hash for rolling update annotations)
	configHash, err := r.ensureDashboardConfigMap(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to ensure ConfigMap: %w", err)
	}

	// Ensure Deployment
	if err := r.ensureDashboardDeployment(ctx, server, configHash); err != nil {
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

// ensureDashboardConfigMap ensures the Dashboard ConfigMap exists.
// Returns the config content hash for use in pod template annotations.
func (r *AxonOpsServerReconciler) ensureDashboardConfigMap(ctx context.Context, server *corev1alpha1.AxonOpsServer) (string, error) {
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

	configData := map[string]string{"axon-dash.yml": configYAML}
	contentHash := configStringDataHash(configData)

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
		cm.Data = configData

		return nil
	})

	if err != nil {
		return "", err
	}

	log.Info("ConfigMap reconciled", "name", name, "operation", op)
	return contentHash, nil
}

// ensureDashboardDeployment ensures the Dashboard Deployment exists.
// configHash is the pre-computed hash of the ConfigMap content.
func (r *AxonOpsServerReconciler) ensureDashboardDeployment(ctx context.Context, server *corev1alpha1.AxonOpsServer, configHash string) error {
	log := logf.FromContext(ctx)
	name := fmt.Sprintf("%s-%s", server.Name, componentDashboard)
	configMapName := name

	// Get component config
	dash := server.Spec.Dashboard

	// Determine image
	image := resolveImage(defaultDashboardImage, server.Spec.ImageRegistry, dash.Repository.Image)
	tag := defaultDashboardTag
	pullPolicy := corev1.PullIfNotPresent
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

	podAnnotations := mergeAnnotationsWithChecksum(dash.Annotations, "checksum/config", configHash)

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
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					NodeSelector:       dash.NodeSelector,
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

// resolveInitImage returns the init container image to use.
// Precedence: spec.initImage > spec.imageRegistry applied to default > default.
func resolveInitImage(server *corev1alpha1.AxonOpsServer) string {
	return resolveImage(defaultInitImage, server.Spec.ImageRegistry, server.Spec.InitImage)
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
