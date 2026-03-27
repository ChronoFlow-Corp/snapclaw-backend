package billing

import (
	"errors"

	"shared/pkg/observability"
)

var (
	ErrInsufficientBalance        = errors.New("billing: insufficient balance")
	ErrPlanInactive               = errors.New("billing: plan inactive")
	ErrSubscriptionNotFound       = errors.New("billing: subscription not found")
	ErrSubscriptionNotPending     = errors.New("billing: subscription not pending")
	ErrPaymentNotPaid             = errors.New("billing: payment not paid")
	ErrPaymentConfirmationMissing = errors.New("billing: payment confirmation missing")
	ErrPaymentSubscriptionMissing = errors.New("billing: payment subscription missing")
	ErrPaymentEventTypeInvalid    = errors.New("billing: payment event type invalid")
	ErrOpenRouterAPIKeyInvalid    = errors.New("billing: openrouter api key invalid")
	ErrUnsupportedCurrency        = errors.New("billing: unsupported currency")
)

func decorateValidation(err error) error {
	return observability.DecorateError(err, observability.ErrorAttrs{
		Result: observability.ResultValidationError,
		Kind:   observability.ErrorKindValidation,
		Source: observability.ErrorSourceService,
	})
}

func decorateExternal(err error) error {
	return observability.DecorateError(err, observability.ErrorAttrs{
		Result: observability.ResultError,
		Kind:   observability.ErrorKindUnexpected,
		Source: observability.ErrorSourceExternal,
	})
}

func decorateInternal(err error) error {
	return observability.DecorateError(err, observability.ErrorAttrs{
		Result: observability.ResultError,
		Kind:   observability.ErrorKindUnexpected,
		Source: observability.ErrorSourceService,
	})
}
