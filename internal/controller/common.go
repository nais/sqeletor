package controller

import (
	"errors"
	"fmt"

	core_v1 "k8s.io/api/core/v1"
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
	errMultipleOwners   = fmt.Errorf("multiple owners: %w", errPermanentFailure)
	errOwnedByOther     = fmt.Errorf("owned by other: %w", errPermanentFailure)
)

func temporaryFailureError(err error) error {
	return fmt.Errorf("%w: %w", errTemporaryFailure, err)
}

func permanentFailureError(err error) error {
	return fmt.Errorf("%w: %w", errPermanentFailure, err)
}

func validateOwnership(ownerReference meta_v1.OwnerReference, secret *core_v1.Secret) error {
	// if we don't manage this resource, error out
	if secret.Labels[managedByKey] != sqeletorFqdnId {
		return fmt.Errorf("secret %s in namespace %s is not managed by us: %w", secret.Name, secret.Namespace, errNotManaged)
	}

	if len(secret.OwnerReferences) > 1 {
		return fmt.Errorf("secret %s in namespace %s has multiple owner references: %w", secret.Name, secret.Namespace, errMultipleOwners)
	}

	if secret.OwnerReferences[0].APIVersion != ownerReference.APIVersion ||
		secret.OwnerReferences[0].Kind != ownerReference.Kind ||
		secret.OwnerReferences[0].Name != ownerReference.Name {
		return fmt.Errorf("secret %s in namespace %s has different owner reference: %w", secret.Name, secret.Namespace, errOwnedByOther)
	}

	return nil
}
