package payment

import (
	"context"
	"fmt"

	"simpleClaw/config"

	"simpleClaw/internal/entities"

	"github.com/google/uuid"
	"github.com/rvinnie/yookassa-sdk-go/yookassa"
	yoocommon "github.com/rvinnie/yookassa-sdk-go/yookassa/common"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

type YooKassa struct {
	env       string
	cl        *yookassa.Client
	returnURL string
}

func NewYooKassa(env string, accountID, secretKey, returnURL string) *YooKassa {
	return &YooKassa{
		env:       env,
		cl:        yookassa.NewClient(accountID, secretKey),
		returnURL: returnURL,
	}
}

func (y *YooKassa) CreatePayment(
	ctx context.Context,
	amount entities.Amount,
	userID uuid.UUID,
	saveMethod bool,
	paymentMethodID *uuid.UUID,
) (entities.Payment, error) {
	const op = "payment.CreatePayment"

	h := yookassa.NewPaymentHandler(y.cl)

	pObj := &yoopayment.Payment{
		Amount: &yoocommon.Amount{
			Value:    amount.Value,
			Currency: amount.Currency,
		},
		SavePaymentMethod: saveMethod,
		Confirmation: entities.Confirmation{
			Type:      entities.ConfirmationTypeRedirect,
			ReturnURL: &y.returnURL,
		},
		MerchantCustomerID: userID.String(),
	}

	if paymentMethodID != nil {
		pObj.PaymentMethodID = paymentMethodID.String()
	}

	if y.env == config.EnvDevelopment {
		pObj.Test = true
	}

	p, err := h.CreatePayment(ctx, pObj)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	pNew, err := mapPayment(p)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	return pNew, nil
}

func (y *YooKassa) Capture(
	ctx context.Context,
	payment *entities.Payment,
) (entities.Payment, error) {
	const op = "payment.Capture"

	h := yookassa.NewPaymentHandler(y.cl)

	p, err := h.CapturePayment(ctx, mapPaymentBack(payment))
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	pNew, err := mapPayment(p)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}
	pNew.UserID = payment.UserID
	pNew.SubscriptionID = payment.SubscriptionID
	pNew.Purpose = payment.Purpose

	return pNew, nil
}

func (y *YooKassa) Cancel(ctx context.Context, paymentID string) (entities.Payment, error) {
	const op = "payment.Cancel"
	h := yookassa.NewPaymentHandler(y.cl)

	p, err := h.CancelPayment(ctx, paymentID)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	return mapPayment(p)
}
