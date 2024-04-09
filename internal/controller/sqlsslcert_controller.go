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

const (
	deploymentCorrelationIdKey = "nais.io/deploymentCorrelationID"
	managedByKey               = "app.kubernetes.io/managed-by"
	typeKey                    = "type"
	appKey                     = "app"
	teamKey                    = "team"

	certKey         = "cert.pem"
	privateKeyKey   = "private-key.pem"
	serverCaCertKey = "server-ca-cert.pem"

	sqeletorFqdnId = "sqeletor.nais.io"
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

func (r *SQLSSLCertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	logger := r.Logger.WithValues("request", req)

	logger.Info("Reconciling SQLSSLCert")

	sqlSslCert := &v1beta1.SQLSSLCert{}
	err := r.Client.Get(ctx, req.NamespacedName, sqlSslCert)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SQLSSLCert not found", "sqlsslcert", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get SQLSSLCert")
		return requeue(), err
	}

	if sqlSslCert.Status.Cert == nil || sqlSslCert.Status.PrivateKey == nil || sqlSslCert.Status.ServerCaCert == nil {
		return requeue(), fmt.Errorf("missing certificate data")
	}

	var secretName string
	var ok bool
	if secretName, ok = sqlSslCert.GetAnnotations()["sqeletor.nais.io/secret-name"]; !ok {
		err = fmt.Errorf("secret name not found")
		logger.Error(nil, "Secret name not found")
		return ctrl.Result{}, err
	}
	logger = logger.WithValues("secret", secretName)

	secret := &v1.Secret{}
	namespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      secretName,
	}
	err = r.Client.Get(ctx, namespacedName, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Secret not found, creating")
			return r.createSecret(ctx, namespacedName, sqlSslCert)
		}
		return requeue(), err
	}
	logger.Info("Secret found, updating")
	return r.updateSecret(ctx, secret, sqlSslCert)
}

func (r *SQLSSLCertReconciler) createSecret(ctx context.Context, namespacedName client.ObjectKey, sqlSslCert *v1beta1.SQLSSLCert) (ctrl.Result, error) {
	secret := &v1.Secret{
		ObjectMeta: v12.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
			Annotations: map[string]string{
				deploymentCorrelationIdKey: sqlSslCert.GetAnnotations()[deploymentCorrelationIdKey],
			},
			Labels: map[string]string{
				managedByKey: sqeletorFqdnId,
				typeKey:      sqeletorFqdnId,
				appKey:       sqlSslCert.GetLabels()[appKey],
				teamKey:      sqlSslCert.GetLabels()[teamKey],
			},
		},
		StringData: map[string]string{
			certKey:         *sqlSslCert.Status.Cert,
			privateKeyKey:   *sqlSslCert.Status.PrivateKey,
			serverCaCertKey: *sqlSslCert.Status.ServerCaCert,
		},
	}
	err := r.Create(ctx, secret)
	if err != nil {
		return requeue(), err
	}
	return ctrl.Result{}, nil
}

func (r *SQLSSLCertReconciler) updateSecret(ctx context.Context, secret *v1.Secret, sqlSslCert *v1beta1.SQLSSLCert) (ctrl.Result, error) {
	// Update annotations
	annotations := secret.GetAnnotations()
	annotations[deploymentCorrelationIdKey] = sqlSslCert.GetAnnotations()[deploymentCorrelationIdKey]

	// Update labels
	labels := secret.GetLabels()
	labels[managedByKey] = sqeletorFqdnId
	labels[typeKey] = sqeletorFqdnId
	labels[appKey] = sqlSslCert.GetLabels()[appKey]
	labels[teamKey] = sqlSslCert.GetLabels()[teamKey]

	// Update data
	secret.StringData[certKey] = *sqlSslCert.Status.Cert
	secret.StringData[privateKeyKey] = *sqlSslCert.Status.PrivateKey
	secret.StringData[serverCaCertKey] = *sqlSslCert.Status.ServerCaCert

	err := r.Update(ctx, secret)
	if err != nil {
		return requeue(), err
	}
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
