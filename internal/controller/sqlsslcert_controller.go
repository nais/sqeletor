package controller

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	certKey      = "cert.pem"
	pk1PemKeyKey = "key.pem"
	pk8DerKeyKey = "key.pk8"
	rootCertKey  = "root-cert.pem"
)

var requeuesMetric = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "sqlsslcert_requeues",
	Help: "Number of requeues for SQLSSLCert",
})

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
			logger.Info("SQLSSLCert not found, aborting reconcile")
			return nil
		}
		return temporaryFailureError(fmt.Errorf("failed to get SQLSSLCert: %w", err))
	}

	secretName, ok := sqlSslCert.Annotations["sqeletor.nais.io/secret-name"]
	if !ok {
		logger.V(4).Info("ignoring: secret name annotation not found")
		return nil
	}
	logger = logger.WithValues("secret", secretName)

	logger.Info("Reconciling SQLSSLCert")

	if sqlSslCert.Status.Cert == nil || sqlSslCert.Status.PrivateKey == nil || sqlSslCert.Status.ServerCaCert == nil {
		err := fmt.Errorf("cert not ready: status.cert: %t, status.privateKey: %t, status.serverCaCert: %t",
			sqlSslCert.Status.Cert != nil,
			sqlSslCert.Status.PrivateKey != nil,
			sqlSslCert.Status.ServerCaCert != nil,
		)
		return temporaryFailureError(err)
	}

	secret := &core_v1.Secret{ObjectMeta: meta_v1.ObjectMeta{Namespace: req.Namespace, Name: secretName}}
	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		if secret.Annotations == nil {
			secret.Annotations = make(map[string]string)
		}

		ownerReference := meta_v1.OwnerReference{
			APIVersion: sqlSslCert.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       sqlSslCert.GetObjectKind().GroupVersionKind().Kind,
			Name:       sqlSslCert.GetName(),
			UID:        sqlSslCert.GetUID(),
		}

		// if new resource, add owner reference and managed-by label.
		// the secret is owned by the sql ssl cert resource.
		if secret.CreationTimestamp.IsZero() {
			secret.OwnerReferences = []meta_v1.OwnerReference{ownerReference}
			secret.Labels[managedByKey] = sqeletorFqdnId
		} else if err := validateOwnership(ownerReference, secret); err != nil {
			return err
		}

		secret.Labels[typeKey] = sqeletorFqdnId
		secret.Labels[appKey] = sqlSslCert.Labels[appKey]
		secret.Labels[teamKey] = sqlSslCert.Labels[teamKey]

		secret.Annotations[deploymentCorrelationIdKey] = sqlSslCert.Annotations[deploymentCorrelationIdKey]
		secret.Annotations[lastUpdatedAnnotation] = time.Now().Format(time.RFC3339)

		derKey, err := pemToPkcs8Der(*sqlSslCert.Status.PrivateKey)
		if err != nil {
			logger.Info("Failed to convert cert to DER", "error", err)
		}
		secret.Data = map[string][]byte{
			pk8DerKeyKey: derKey,
		}
		secret.StringData = map[string]string{
			certKey:      *sqlSslCert.Status.Cert,
			pk1PemKeyKey: *sqlSslCert.Status.PrivateKey,
			rootCertKey:  *sqlSslCert.Status.ServerCaCert,
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, errPermanentFailure) {
			return err
		}
		return temporaryFailureError(err)
	}

	logger.Info("Secret reconciled", "operation", op)
	return nil
}

func (r *SQLSSLCertReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLSSLCert{}).
		Complete(r)
}

func decodePrivateKeyPem(in []byte) ([]byte, error) {
	for {
		var block *pem.Block
		block, in = pem.Decode(in)
		if block == nil {
			return nil, errors.New("failed to decode PEM block")
		}
		if block.Type == "RSA PRIVATE KEY" {
			return block.Bytes, nil
		}
	}
}

func pemToPkcs8Der(pem string) ([]byte, error) {
	der, err := decodePrivateKeyPem([]byte(pem))
	if err != nil {
		return nil, err
	}

	rsaKey, err := x509.ParsePKCS1PrivateKey(der)
	if err != nil {
		return nil, err
	}

	pkcs8WrappedRsaKey, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		return nil, err
	}

	return pkcs8WrappedRsaKey, nil
}
