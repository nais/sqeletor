package controller

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"time"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var requeues = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "sqlsslcert_requeues",
	Help: "Number of requeues for SQLSSLCert",
})

func init() {
	metrics.Registry.MustRegister(requeues)
}

// SQLSSLCertReconciler reconciles a SQLSSLCert object
type SQLSSLCertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
}

//+kubebuilder:rbac:groups=sql.cnrm.cloud.google.com,resources=sqlsslcerts,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=sql.cnrm.cloud.google.com,resources=sqlsslcerts/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SQLSSLCert object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *SQLSSLCertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	r.Logger.Info("Reconciling SQLSSLCert", "request", req)

	sqlSslCert := &v1beta1.SQLSSLCert{}
	err := r.Client.Get(ctx, req.NamespacedName, sqlSslCert)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Logger.Info("SQLSSLCert not found", "sqlsslcert", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		r.Logger.Error(err, "Failed to get SQLSSLCert")
		requeues.Inc()
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, nil
	}
	r.Logger.Info("Got SQLSSLCert", "sqlsslcert", sqlSslCert.Status.Cert)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SQLSSLCertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLSSLCert{}).
		Complete(r)
}
