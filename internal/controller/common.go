package controller

import (
	"errors"
	"fmt"
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
)

func temporaryFailureError(err error) error {
	return fmt.Errorf("%w: %w", errTemporaryFailure, err)
}

func permanentFailureError(err error) error {
	return fmt.Errorf("%w: %w", errPermanentFailure, err)
}
