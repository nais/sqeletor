package controller

import (
	"errors"
	"fmt"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	deploymentCorrelationIdKey = "nais.io/deploymentCorrelationID"
	managedByKey               = "app.kubernetes.io/managed-by"
	typeKey                    = "type"
	appKey                     = "app"
	teamKey                    = "team"

	sqeletorFqdnId = "sqeletor.nais.io"
)

var (
	errTemporaryFailure = errors.New("temporary failure")
	errPermanentFailure = errors.New("permanent failure")
	errNotManaged       = fmt.Errorf("not managed by controller: %w", errPermanentFailure)
	errNoOwner          = fmt.Errorf("no owner: %w", errPermanentFailure)
	errMultipleOwners   = fmt.Errorf("multiple owners: %w", errPermanentFailure)
	errOwnedByOther     = fmt.Errorf("owned by other: %w", errPermanentFailure)
)

func temporaryFailureError(err error) error {
	return fmt.Errorf("%w: %w", errTemporaryFailure, err)
}

func permanentFailureError(err error) error {
	return fmt.Errorf("%w: %w", errPermanentFailure, err)
}

func validateOwnership(ownerReference meta_v1.OwnerReference, meta meta_v1.Object) error {
	// if we don't manage this resource, error out
	if meta.GetLabels()[managedByKey] != sqeletorFqdnId {
		return fmt.Errorf("resource %s in namespace %s is not managed by us: %w", meta.GetName(), meta.GetNamespace(), errNotManaged)
	}

	ownerReferences := meta.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		return fmt.Errorf("resource %s in namespace %s does not have any owner reference: %w", meta.GetName(), meta.GetNamespace(), errNoOwner)
	}
	if len(ownerReferences) > 1 {
		return fmt.Errorf("resource %s in namespace %s has multiple owner references: %w", meta.GetName(), meta.GetNamespace(), errMultipleOwners)
	}

	if ownerReferences[0].APIVersion != ownerReference.APIVersion ||
		ownerReferences[0].Kind != ownerReference.Kind ||
		ownerReferences[0].Name != ownerReference.Name {
		return fmt.Errorf("resource %s in namespace %s has different owner reference: %w", meta.GetName(), meta.GetNamespace(), errOwnedByOther)
	}

	return nil
}
