package controller

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
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

var (
	requeuesMetric = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sqlsslcert_requeues",
		Help: "Number of requeues for SQLSSLCert",
	})

	errTemporaryFailure = errors.New("temporary failure")
	errNotOwner         = errors.New("not owner")
)

func init() {
	metrics.Registry.MustRegister(requeuesMetric)
}

// SQLSSLCertReconciler reconciles a SQLSSLCert object
type SQLSSLCertReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SQLSSLCertReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling SQLSSLCert")

	err := r.reconcileSQLSSLCert(ctx, req)
	if errors.Is(err, errTemporaryFailure) {
		requeuesMetric.Inc()
		logger.Error(err, "requeueing after temporary failure")
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, nil
	}
	return ctrl.Result{}, err
}

func (r *SQLSSLCertReconciler) reconcileSQLSSLCert(ctx context.Context, req ctrl.Request) error {
	logger := log.FromContext(ctx)

	sqlSslCert := &v1beta1.SQLSSLCert{}
	if err := r.Client.Get(ctx, req.NamespacedName, sqlSslCert); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SQLSSLCert not found, aborting reconcile", "sqlsslcert", req.NamespacedName)
			return nil
		}
		logger.Error(err, "Failed to get SQLSSLCert")
		return temporaryFailureError(err)
	}

	if sqlSslCert.Status.Cert == nil || sqlSslCert.Status.PrivateKey == nil || sqlSslCert.Status.ServerCaCert == nil {
		err := fmt.Errorf("cert not ready: status.cert: %t, status.privateKey: %t, status.serverCaCert: %t",
			sqlSslCert.Status.Cert != nil,
			sqlSslCert.Status.PrivateKey != nil,
			sqlSslCert.Status.ServerCaCert != nil,
		)
		return temporaryFailureError(err)
	}

	var secretName string
	var ok bool
	if secretName, ok = sqlSslCert.GetAnnotations()["sqeletor.nais.io/secret-name"]; !ok {
		return fmt.Errorf("secret name not found")
	}
	logger = logger.WithValues("secret", secretName)

	secret := &core_v1.Secret{ObjectMeta: meta_v1.ObjectMeta{Namespace: req.Namespace, Name: secretName}}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		// if new resource, add ourselves as owner
		if secret.CreationTimestamp.IsZero() {
			secret.OwnerReferences = []meta_v1.OwnerReference{}
		}

		// if we don't own this resource, error out
		for _, ref := range secret.OwnerReferences {
			if ref.Name != sqeletorFqdnId {
				return fmt.Errorf("secret %s in namesapce %s is not owned by %s: %w", secret.Name, secret.Namespace, sqeletorFqdnId, errNotOwner)
			}
		}

		annotations := secret.GetAnnotations()
		annotations[deploymentCorrelationIdKey] = sqlSslCert.GetAnnotations()[deploymentCorrelationIdKey]

		labels := secret.GetLabels()
		labels[managedByKey] = sqeletorFqdnId
		labels[typeKey] = sqeletorFqdnId
		labels[appKey] = sqlSslCert.GetLabels()[appKey]
		labels[teamKey] = sqlSslCert.GetLabels()[teamKey]

		secret.StringData = map[string]string{
			certKey:         *sqlSslCert.Status.Cert,
			privateKeyKey:   *sqlSslCert.Status.PrivateKey,
			serverCaCertKey: *sqlSslCert.Status.ServerCaCert,
		}

		return nil
	})
	if err != nil {
		if !errors.Is(err, errNotOwner) {
			return temporaryFailureError(err)
		}
		return err
	}

	logger.Info("Secret reconciled", "operation", op)
	return nil
}

func (r *SQLSSLCertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLSSLCert{}).
		Complete(r)
}

func temporaryFailureError(err error) error {
	return fmt.Errorf("%w: %w", errTemporaryFailure, err)
}
