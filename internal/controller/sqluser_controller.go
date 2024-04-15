package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"time"

	nais_io_v1alpha1 "github.com/nais/liberator/pkg/apis/nais.io/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const ()

var (
	userRequeuesMetric = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "sqluser_requeues",
		Help: "Number of requeues for SQLUser",
	})

	errOwnedByOtherUser = fmt.Errorf("owned by other user: %w", errPermanentFailure)
)

func init() {
	metrics.Registry.MustRegister(requeuesMetric)
}

// SQLUserReconciler reconciles a SQLUser object
type SQLUserReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SQLUserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling SQLUser")

	err := r.reconcileSQLUser(ctx, req)
	if errors.Is(err, errTemporaryFailure) {
		userRequeuesMetric.Inc()
		logger.Error(err, "requeueing after temporary failure")
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, nil
	}
	return ctrl.Result{}, err
}

func validateSecretKeyRef(sqlUser *v1beta1.SQLUser) error {
	if sqlUser.Spec.Password == nil ||
		sqlUser.Spec.Password.ValueFrom == nil ||
		sqlUser.Spec.Password.ValueFrom.SecretKeyRef == nil ||
		sqlUser.Spec.Password.ValueFrom.SecretKeyRef.Key == "" ||
		sqlUser.Spec.Password.ValueFrom.SecretKeyRef.Name == "" {
		return fmt.Errorf("password secret ref not properly set")
	}
	return nil
}

func (r *SQLUserReconciler) getInstancePrivateIP(ctx context.Context, key types.NamespacedName) (string, error) {
	sqlInstance := &v1beta1.SQLInstance{}
	if err := r.Client.Get(ctx, key, sqlInstance); err != nil {
		return "", temporaryFailureError(fmt.Errorf("failed to get SQLUser: %w", err))
	}
	if sqlInstance.Spec.Settings.IpConfiguration.PrivateNetworkRef == nil {
		return "", permanentFailureError(fmt.Errorf("referenced sql instance is not configured for private ip"))
	}
	if sqlInstance.Status.PrivateIpAddress == nil || *sqlInstance.Status.PrivateIpAddress == "" {
		return "", temporaryFailureError(fmt.Errorf("referenced sql instance does not have a private ip"))
	}
	return *sqlInstance.Status.PrivateIpAddress, nil
}

func (r *SQLUserReconciler) reconcileSQLUser(ctx context.Context, req ctrl.Request) error {
	logger := log.FromContext(ctx)

	sqlUser := &v1beta1.SQLUser{}
	if err := r.Client.Get(ctx, req.NamespacedName, sqlUser); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SQLUser not found, aborting reconcile")
			return nil
		}
		return temporaryFailureError(fmt.Errorf("failed to get SQLUser: %w", err))
	}

	if err := validateSecretKeyRef(sqlUser); err != nil {
		return permanentFailureError(err)
	}
	secretName := sqlUser.Spec.Password.ValueFrom.SecretKeyRef.Name
	secretKey := sqlUser.Spec.Password.ValueFrom.SecretKeyRef.Key
	logger = logger.WithValues("secretName", secretName, "secretKey", secretKey)

	envVarPrefix, ok := sqlUser.Annotations["sqeletor.nais.io/env-var-prefix"]
	if !ok {
		return fmt.Errorf("env var prefix annotation not found")
	}
	dbName, ok := sqlUser.Annotations["sqeletor.nais.io/database-name"]
	if !ok {
		return fmt.Errorf("database name annotation not found")
	}

	logger = logger.WithValues("envVarPrefix", envVarPrefix, "databaseName", dbName)

	instanceKey := types.NamespacedName{Name: sqlUser.Spec.InstanceRef.Name, Namespace: sqlUser.Spec.InstanceRef.Namespace}
	instanceIP, err := r.getInstancePrivateIP(ctx, instanceKey)
	if err != nil {
		return err
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
			APIVersion: sqlUser.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       sqlUser.GetObjectKind().GroupVersionKind().Kind,
			Name:       sqlUser.GetName(),
			UID:        sqlUser.GetUID(),
		}
		if err := validateOwnership(ownerReference, secret); err != nil {
			return err
		}

		// if new resource, add owner reference and managed-by label
		if secret.CreationTimestamp.IsZero() {
			secret.OwnerReferences = []meta_v1.OwnerReference{ownerReference}
			secret.Labels[managedByKey] = sqeletorFqdnId
		}

		secret.Labels[typeKey] = sqeletorFqdnId
		secret.Labels[appKey] = sqlUser.Labels[appKey]
		secret.Labels[teamKey] = sqlUser.Labels[teamKey]

		secret.Annotations[deploymentCorrelationIdKey] = sqlUser.Annotations[deploymentCorrelationIdKey]

		password := generatePassword()
		prefixedPasswordKey := envVarPrefix + "_PASSWORD"
		if secretKey != prefixedPasswordKey {
			return permanentFailureError(fmt.Errorf("secret key %s does not match expected key %s", secretKey, prefixedPasswordKey))
		}

		postgresPort := "5432"

		rootCertPath := filepath.Join(nais_io_v1alpha1.DefaultSqeletorMountPath, serverCaCertKey)
		certPath := filepath.Join(nais_io_v1alpha1.DefaultSqeletorMountPath, certKey)
		privateKeyPath := filepath.Join(nais_io_v1alpha1.DefaultSqeletorMountPath, privateKeyKey)

		queries := url.Values{}
		queries.Add("sslmode", "verify-ca")
		queries.Add("sslcert", certPath)
		queries.Add("sslkey", privateKeyPath)
		queries.Add("sslrootcert", rootCertPath)
		googleSQLPostgresURL := url.URL{
			Scheme:   "postgresql",
			Path:     dbName,
			User:     url.UserPassword(sqlUser.Name, password),
			Host:     net.JoinHostPort(instanceIP, postgresPort),
			RawQuery: queries.Encode(),
		}

		secret.StringData = map[string]string{
			prefixedPasswordKey:        password,
			envVarPrefix + "_HOST":     instanceIP,
			envVarPrefix + "_PORT":     postgresPort,
			envVarPrefix + "_DATABASE": dbName,
			envVarPrefix + "_USERNAME": sqlUser.Name,
			envVarPrefix + "_URL":      googleSQLPostgresURL.String(),

			"PGSSLMODE":     "verify-ca",
			"PGSSLROOTCERT": rootCertPath,
			"PGSSLCERT":     certPath,
			"PGSSLKEY":      privateKeyPath,
			"PGHOSTADDR":    instanceIP,
			"PGPORT":        postgresPort,
			"PGPASSWORD":    password,
			"PGDATABASE":    dbName,
			"PGUSER":        sqlUser.Name,
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

func (r *SQLUserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLUser{}).
		Complete(r)
}

func generatePassword() string {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	if err != nil {
		panic(err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(buf)
}