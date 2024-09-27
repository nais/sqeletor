package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/GoogleCloudPlatform/k8s-config-connector/pkg/clients/generated/apis/sql/v1beta1"
	"github.com/prometheus/client_golang/prometheus"
	netv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var instanceRequeuesMetric = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "sqlinstance_requeues",
	Help: "Number of requeues for SQLInstance",
})

var ipTypesToKeep = []string{"PRIMARY", "PRIVATE"}

func init() {
	metrics.Registry.MustRegister(instanceRequeuesMetric)
}

// SQLInstanceReconciler reconciles a SQLInstance object
type SQLInstanceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *SQLInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	err := r.reconcile(ctx, req)
	if errors.Is(err, errTemporaryFailure) {
		instanceRequeuesMetric.Inc()
		logger.Error(err, "requeueing after temporary failure")
		return ctrl.Result{
			RequeueAfter: time.Minute,
		}, nil
	}
	if err != nil {
		logger.Error(err, "failed to reconcile SQLInstance")
	}
	return ctrl.Result{}, err
}

func (r *SQLInstanceReconciler) reconcile(ctx context.Context, req ctrl.Request) error {
	logger := log.FromContext(ctx)

	sqlInstance := &v1beta1.SQLInstance{}
	if err := r.Get(ctx, req.NamespacedName, sqlInstance); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("SQLInstance not found, aborting reconcile")
			return nil
		}
		return temporaryFailureError(fmt.Errorf("failed to get SQLInstance: %w", err))
	}

	if sqlInstance.Spec.ResourceID == nil {
		logger.Info("SQLInstance has no resource ID, requeueing")
		return temporaryFailureError(fmt.Errorf("SQLInstance has no resource ID"))
	}

	ips := []string{}
	for _, ip := range sqlInstance.Status.IpAddress {
		if ip.IpAddress != nil && slices.Contains(ipTypesToKeep, ptr.Deref(ip.Type, "")) {
			ips = append(ips, ptr.Deref(ip.IpAddress, ""))
		}
	}
	if len(ips) == 0 {
		logger.Info("SQLInstance has no IP address, requeueing")
		return temporaryFailureError(fmt.Errorf("SQLInstance has no IP address"))
	}

	netpol := &netv1.NetworkPolicy{
		ObjectMeta: meta_v1.ObjectMeta{
			Name:      "sql-" + sqlInstance.Name + "-" + *sqlInstance.Spec.ResourceID,
			Namespace: sqlInstance.Namespace,
		},
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, netpol, func() error {
		if netpol.Labels == nil {
			netpol.Labels = make(map[string]string)
		}
		if netpol.Annotations == nil {
			netpol.Annotations = make(map[string]string)
		}

		ownerReference := meta_v1.OwnerReference{
			APIVersion: sqlInstance.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Kind:       sqlInstance.GetObjectKind().GroupVersionKind().Kind,
			Name:       sqlInstance.GetName(),
			UID:        sqlInstance.GetUID(),
		}

		// if new resource, add owner reference and managed-by label
		// the netpol is owned by the sql instance.
		if netpol.CreationTimestamp.IsZero() {
			netpol.OwnerReferences = []meta_v1.OwnerReference{ownerReference}
			netpol.Labels[managedByKey] = sqeletorFqdnId
		} else if err := validateOwnership(ownerReference, netpol); err != nil {
			return err
		}

		netpol.Labels[typeKey] = sqeletorFqdnId
		netpol.Labels[appKey] = sqlInstance.Labels[appKey]
		netpol.Labels[teamKey] = sqlInstance.Labels[teamKey]

		netpol.Annotations[deploymentCorrelationIdKey] = sqlInstance.Annotations[deploymentCorrelationIdKey]

		netpol.Spec.PodSelector = meta_v1.LabelSelector{
			MatchLabels: map[string]string{
				appKey: sqlInstance.Labels[appKey],
			},
		}

		netpol.Spec.PolicyTypes = []netv1.PolicyType{netv1.PolicyTypeEgress}
		netpol.Spec.Egress = []netv1.NetworkPolicyEgressRule{}
		slices.Sort(ips)
		for _, ip := range ips {
			netpol.Spec.Egress = append(netpol.Spec.Egress, netv1.NetworkPolicyEgressRule{
				To: []netv1.NetworkPolicyPeer{
					{
						IPBlock: &netv1.IPBlock{
							CIDR: ip + "/32",
						},
					},
				},
			})
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, errPermanentFailure) {
			return err
		}
		return temporaryFailureError(err)
	}

	logger.Info("Netpol reconciled", "operation", op)
	return nil
}

func (r *SQLInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SQLInstance{}).
		Complete(r)
}
