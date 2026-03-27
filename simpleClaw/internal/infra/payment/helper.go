package payment

import (
	"encoding/json"
	"errors"
	"time"

	"simpleClaw/internal/entities"

	yoocommon "github.com/rvinnie/yookassa-sdk-go/yookassa/common"
	yoopayment "github.com/rvinnie/yookassa-sdk-go/yookassa/payment"
)

func mapPayment(old entities.Payment, in *yoopayment.Payment) (entities.Payment, error) {
	if in == nil {
		return entities.Payment{}, errors.New("nil payment")
	}

	var conf entities.Confirmation

	raw, err := json.Marshal(in.Confirmation)
	if err != nil {
		return entities.Payment{}, err
	}

	err = json.Unmarshal(raw, &conf)
	if err != nil {
		return entities.Payment{}, err
	}

	return entities.Payment{
		ID:                   in.ID,
		UserID:               old.UserID,
		SubscriptionID:       old.SubscriptionID,
		Status:               mapStatus(in.Status),
		Paid:                 in.Paid,
		Amount:               mapAmountValue(in.Amount),
		AuthorizationDetails: mapAuthorizationDetails(in.AuthorizationDetails),
		CreatedAt:            mapTimeValue(in.CreatedAt),
		Description:          in.Description,
		Confirmation:         conf,
		ExpiresAt:            mapTimePtr(in.ExpiresAt),
		Metadata:             in.Metadata,
		PaymentMethod:        mapPaymentMethod(in.PaymentMethod),
		Recipient:            mapRecipient(in.Recipient),
		Refundable:           in.Refundable,
		Test:                 in.Test,
		IncomeAmount:         mapAmountPtr(in.IncomeAmount),
	}, nil
}

func mapStatus(s yoopayment.Status) entities.Status {
	switch string(s) {
	case string(entities.Pending):
		return entities.Pending
	case string(entities.WaitingForCapture):
		return entities.WaitingForCapture
	case string(entities.Succeeded):
		return entities.Succeeded
	case string(entities.Canceled):
		return entities.Canceled
	default:
		return entities.Status(s)
	}
}

func mapAmountValue(in *yoocommon.Amount) entities.Amount {
	if in == nil {
		return entities.Amount{}
	}

	return entities.Amount{
		Value:    in.Value,
		Currency: in.Currency,
	}
}

func mapAmountPtr(in *yoocommon.Amount) *entities.Amount {
	if in == nil {
		return nil
	}

	return &entities.Amount{
		Value:    in.Value,
		Currency: in.Currency,
	}
}

func mapAuthorizationDetails(in *yoopayment.AuthorizationDetails) *entities.AuthorizationDetails {
	if in == nil {
		return nil
	}

	return &entities.AuthorizationDetails{
		RRN:      in.RRN,
		AuthCode: in.AuthCode,
		ThreeDSecure: &entities.ThreeDSecure{
			Applied: in.ThreeDSecure.Applied,
		},
	}
}

func mapRecipient(in *yoopayment.Recipient) *entities.Recipient {
	if in == nil {
		return nil
	}

	return &entities.Recipient{
		AccountID: in.AccountId,
		GatewayID: in.GatewayId,
	}
}

func mapTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}

	t := *in
	return &t
}

func mapTimeValue(in *time.Time) time.Time {
	if in == nil {
		return time.Time{}
	}

	return *in
}

func mapPaymentMethod(in yoopayment.PaymentMethoder) *entities.PaymentMethodDetails {
	if in == nil {
		return nil
	}

	raw, err := json.Marshal(in)
	if err != nil {
		return nil
	}

	var out entities.PaymentMethodDetails
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}

	return &out
}

func mapPaymentBack(in *entities.Payment) *yoopayment.Payment {
	if in == nil {
		return nil
	}

	return &yoopayment.Payment{
		ID:                   in.ID,
		Status:               mapStatusBack(in.Status),
		Amount:               mapAmountBackValue(in.Amount),
		IncomeAmount:         mapAmountBackPtr(in.IncomeAmount),
		Description:          in.Description,
		Recipient:            mapRecipientBack(in.Recipient),
		PaymentMethod:        in.PaymentMethod,
		CreatedAt:            mapTimeBackValue(in.CreatedAt),
		ExpiresAt:            mapTimeBackPtr(in.ExpiresAt),
		Test:                 in.Test,
		Paid:                 in.Paid,
		Refundable:           in.Refundable,
		Metadata:             in.Metadata,
		AuthorizationDetails: mapAuthorizationDetailsBack(in.AuthorizationDetails),
	}
}

func mapStatusBack(s entities.Status) yoopayment.Status {
	return yoopayment.Status(s)
}

func mapAmountBackValue(in entities.Amount) *yoocommon.Amount {
	return &yoocommon.Amount{
		Value:    in.Value,
		Currency: in.Currency,
	}
}

func mapAmountBackPtr(in *entities.Amount) *yoocommon.Amount {
	if in == nil {
		return nil
	}

	return &yoocommon.Amount{
		Value:    in.Value,
		Currency: in.Currency,
	}
}

func mapAuthorizationDetailsBack(
	in *entities.AuthorizationDetails,
) *yoopayment.AuthorizationDetails {
	if in == nil {
		return nil
	}

	return &yoopayment.AuthorizationDetails{
		RRN:      in.RRN,
		AuthCode: in.AuthCode,
		ThreeDSecure: struct {
			Applied bool `json:"applied,omitempty"`
		}{Applied: in.ThreeDSecure.Applied},
	}
}

func mapRecipientBack(in *entities.Recipient) *yoopayment.Recipient {
	if in == nil {
		return nil
	}

	return &yoopayment.Recipient{
		AccountId: in.AccountID,
		GatewayId: in.GatewayID,
	}
}

func mapTimeBackPtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}

	t := *in
	return &t
}

func mapTimeBackValue(in time.Time) *time.Time {
	if in.IsZero() {
		return nil
	}

	t := in
	return &t
}
