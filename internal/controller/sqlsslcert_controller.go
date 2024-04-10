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

var (
	requeuesMetric = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sqlsslcert_requeues",
		Help: "Number of requeues for SQLSSLCert",
	})

	temporaryFailure = errors.New("temporary failure")
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
	if errors.Is(err, temporaryFailure) {
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

	namespacedName := client.ObjectKey{
		Namespace: req.Namespace,
		Name:      secretName,
	}
	secret := &core_v1.Secret{}
	if err := r.Client.Get(ctx, namespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Secret not found, creating")
			return r.createSecret(ctx, namespacedName, sqlSslCert, logger)
		}
		return temporaryFailureError(err)
	}

	logger.Info("Secret found, updating")
	return r.updateSecret(ctx, secret, sqlSslCert)
}

func (r *SQLSSLCertReconciler) createSecret(ctx context.Context, namespacedName client.ObjectKey, sqlSslCert *v1beta1.SQLSSLCert, logger logr.Logger) error {
	secret := &core_v1.Secret{
		ObjectMeta: meta_v1.ObjectMeta{
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

	if err := controllerutil.SetOwnerReference(sqlSslCert, secret, r.Scheme); err != nil {
		logger.Error(err, "Failed to set owner reference")
		return err
	}

	if err := r.Create(ctx, secret); err != nil {
		return temporaryFailureError(err)
	}

	return nil
}

func (r *SQLSSLCertReconciler) updateSecret(ctx context.Context, secret *core_v1.Secret, sqlSslCert *v1beta1.SQLSSLCert) error {
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
	secret.StringData = make(map[string]string)
	secret.StringData[certKey] = *sqlSslCert.Status.Cert
	secret.StringData[privateKeyKey] = *sqlSslCert.Status.PrivateKey
	secret.StringData[serverCaCertKey] = *sqlSslCert.Status.ServerCaCert

	if err := r.Update(ctx, secret); err != nil {
		return temporaryFailureError(err)
	}

	return nil
}

func (r *SQLSSLCertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLSSLCert{}).
		Complete(r)
}

func temporaryFailureError(err error) error {
	return fmt.Errorf("%w: %w", temporaryFailure, err)
}
