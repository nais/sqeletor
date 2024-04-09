package controller

import (
	"context"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		return requeue(), err
	}
	r.Logger.Info("Got SQLSSLCert", "sqlsslcert", sqlSslCert.Status.Cert)

	var secretName string
	var ok bool
	if secretName, ok = sqlSslCert.GetAnnotations()["sqeletor.nais.io/secret-name"]; !ok {
		err = fmt.Errorf("secret name not found")
		r.Logger.Error(nil, "Secret name not found")
		return ctrl.Result{}, err
	}
	secret := &v1.Secret{}
	namespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      secretName,
	}
	err = r.Client.Get(ctx, namespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Logger.Info("Secret not found", "secret", secretName)
			return r.createSecret(ctx, namespacedName, sqlSslCert)
		}
		return requeue(), err
	}
	return r.updateSecret(secret, sqlSslCert)
}

func (r *SQLSSLCertReconciler) createSecret(ctx context.Context, namespacedName client.ObjectKey, sqlSslCert *v1beta1.SQLSSLCert) (ctrl.Result, error) {

	if sqlSslCert.Status.Cert == nil || sqlSslCert.Status.PrivateKey == nil || sqlSslCert.Status.ServerCaCert == nil {
		return requeue(), fmt.Errorf("missing certificate data")
	}

	secret := &v1.Secret{
		ObjectMeta: v12.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Annotations: map[string]string{
				"nais.io/deploymentCorrelationID": sqlSslCert.GetAnnotations()["nais.io/deploymentCorrelationID"],
			},
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "sqeletor",
				"type":                         "sqeletor.nais.io",
				"app":                          sqlSslCert.GetLabels()["app"],
				"team":                         sqlSslCert.GetLabels()["team"],
			},
		},
		StringData: map[string]string{
			"cert.pem":           *sqlSslCert.Status.Cert,
			"private-key.pem":    *sqlSslCert.Status.PrivateKey,
			"server-ca-cert.pem": *sqlSslCert.Status.ServerCaCert,
		},
	}
	err := r.Create(ctx, secret)
	if err != nil {
		return requeue(), err
	}
	return ctrl.Result{}, nil
}

func (r *SQLSSLCertReconciler) updateSecret(secret *v1.Secret, status *v1beta1.SQLSSLCert) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SQLSSLCertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLSSLCert{}).
		Complete(r)
}

func requeue() ctrl.Result {
	requeues.Inc()
	return ctrl.Result{
		RequeueAfter: time.Minute,
	}
}
