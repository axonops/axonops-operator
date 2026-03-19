package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// ReconcileDuration measures the duration of reconciliation operations in seconds.
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "axonops_reconciliation_duration_seconds",
			Help:    "Duration of reconciliation in seconds",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
		},
		[]string{"resource_type", "result"},
	)

	// ReconcileTotal counts the total number of reconciliations.
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "axonops_reconciliation_total",
			Help: "Total number of reconciliations",
		},
		[]string{"resource_type", "result"},
	)

	// ResourceCreatedTotal counts resources created by the operator.
	ResourceCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "axonops_resource_created_total",
			Help: "Total resources created",
		},
		[]string{"resource_type"},
	)

	// ResourceUpdatedTotal counts resources updated by the operator.
	ResourceUpdatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "axonops_resource_updated_total",
			Help: "Total resources updated",
		},
		[]string{"resource_type"},
	)

	// ResourceDeletedTotal counts resources deleted by the operator.
	ResourceDeletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "axonops_resource_deleted_total",
			Help: "Total resources deleted",
		},
		[]string{"resource_type"},
	)

	// ReconcileErrorsTotal counts reconciliation errors by type.
	ReconcileErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "axonops_reconciliation_errors_total",
			Help: "Total reconciliation errors by type",
		},
		[]string{"error_type"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		ReconcileDuration,
		ReconcileTotal,
		ResourceCreatedTotal,
		ResourceUpdatedTotal,
		ResourceDeletedTotal,
		ReconcileErrorsTotal,
	)
}

// Metric result label values used by all controllers.
const (
	ResultSuccess = "success"
	ResultError   = "error"
)

// ClassifyError categorizes a Kubernetes API error into a metric label value.
func ClassifyError(err error) string {
	if err == nil {
		return "none"
	}
	switch {
	case errors.IsNotFound(err):
		return "not_found"
	case errors.IsConflict(err):
		return "conflict"
	case errors.IsTimeout(err) || errors.IsServerTimeout(err):
		return "timeout"
	case errors.IsInvalid(err):
		return "validation_error"
	case errors.IsInternalError(err):
		return "api_error"
	default:
		return "other"
	}
}
