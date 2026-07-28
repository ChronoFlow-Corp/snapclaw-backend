package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/infra/sql/models"
)

type Storage struct {
	db *gorm.DB
}

func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) Create(ctx context.Context, payment entities.Payment) error {
	const op = "storages.Payments.Create"

	if err := validatePaymentForCreate(payment); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	paymentModel, err := MapPaymentToModel(payment)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := gorm.G[models.Payment](s.db).Create(ctx, &paymentModel); err != nil {
		return fmt.Errorf("%s: %w", op, translatePaymentError(err))
	}

	return nil
}

func (s *Storage) GetByID(
	ctx context.Context,
	id string,
) (entities.Payment, error) {
	const op = "storages.Payments.GetByID"

	if id == "" {
		return entities.Payment{}, sql.ErrInvalid
	}

	paymentModel, err := gorm.G[models.Payment](s.db).
		Where("id = ?", id).
		First(ctx)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	payment, err := mapModelToPayment(paymentModel)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	return payment, nil
}

func (s *Storage) GetByUserID(ctx context.Context, userID uuid.UUID) ([]entities.Payment, error) {
	const op = "storages.Payments.GetByUserID"

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	paymentModels, err := gorm.G[models.Payment](s.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	payments := make([]entities.Payment, 0, len(paymentModels))
	for _, paymentModel := range paymentModels {
		payment, mapErr := mapModelToPayment(paymentModel)
		if mapErr != nil {
			return nil, fmt.Errorf("%s: %w", op, mapErr)
		}

		payments = append(payments, payment)
	}

	return payments, nil
}

func (s *Storage) GetLatestSucceededByPurpose(
	ctx context.Context,
	userID uuid.UUID,
	purpose entities.PaymentPurpose,
) (entities.Payment, error) {
	const op = "storages.Payments.GetLatestSucceededByPurpose"

	if userID == uuid.Nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	purpose = normalizePurpose(purpose)

	paymentModel, err := gorm.G[models.Payment](s.db).
		Where(
			"user_id = ? AND purpose = ? AND status = ? AND paid = ?",
			userID,
			string(purpose),
			string(entities.Succeeded),
			true,
		).
		Order("created_at DESC").
		First(ctx)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	payment, err := mapModelToPayment(paymentModel)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	return payment, nil
}

func (s *Storage) GetLatestBySubscriptionID(
	ctx context.Context,
	subscriptionID, userID uuid.UUID,
) (entities.Payment, error) {
	const op = "storages.Payments.GetLatestBySubscriptionID"

	switch {
	case subscriptionID == uuid.Nil:
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	case userID == uuid.Nil:
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	paymentModel, err := gorm.G[models.Payment](s.db).
		Where("subscription_id = ? AND user_id = ?", subscriptionID, userID).
		Order("created_at DESC").
		First(ctx)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, sql.TranslateError(err))
	}

	payment, err := mapModelToPayment(paymentModel)
	if err != nil {
		return entities.Payment{}, fmt.Errorf("%s: %w", op, err)
	}

	return payment, nil
}

func (s *Storage) Update(ctx context.Context, payment entities.Payment) error {
	const op = "storages.Payments.Update"

	if err := validatePaymentForCreate(payment); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := gorm.G[models.Payment](tx.
			Clauses(clause.Locking{Strength: "UPDATE"})).
			Where("id = ? AND user_id = ?", payment.ID, payment.UserID).
			First(ctx)
		if err != nil {
			return translatePaymentError(err)
		}

		updateFields, err := buildPaymentUpdateFields(payment)
		if err != nil {
			return err
		}

		updateTx := tx.Model(&models.Payment{}).
			Where("id = ? AND user_id = ?", payment.ID, payment.UserID).
			Updates(updateFields)
		if updateTx.Error != nil {
			return translatePaymentError(updateTx.Error)
		}

		if updateTx.RowsAffected == 0 {
			return sql.ErrNotFound
		}

		return nil
	}); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Storage) Delete(ctx context.Context, id string, userID uuid.UUID) error {
	const op = "storages.Payments.Delete"

	if err := validateIDAndUserID(id, userID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	affected, err := gorm.G[models.Payment](s.db).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", op, translatePaymentError(err))
	}

	if affected == 0 {
		return fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	return nil
}

func validatePaymentIdentity(payment entities.Payment) error {
	return validateIDAndUserID(payment.ID, payment.UserID)
}

func validatePaymentForCreate(payment entities.Payment) error {
	if err := validatePaymentIdentity(payment); err != nil {
		return err
	}

	return validateStoredPayment(payment)
}

func validateStoredPayment(payment entities.Payment) error {
	switch {
	case normalizePurpose(payment.Purpose) == "":
		return invalidPayment("payment purpose is required")
	case payment.Status == "":
		return invalidPayment("payment status is required")
	case payment.Amount.Value == "":
		return invalidPayment("payment amount value is required")
	case payment.Amount.Currency == "":
		return invalidPayment("payment amount currency is required")
	case payment.CreatedAt.IsZero():
		return invalidPayment("payment created_at is required")
	}

	return validateNestedPayment(payment)
}

func validateIDAndUserID(id string, userID uuid.UUID) error {
	switch {
	case id == "":
		return invalidPayment("payment id is required")
	case userID == uuid.Nil:
		return invalidPayment("payment user_id is required")
	default:
		return nil
	}
}

func validateNestedPayment(payment entities.Payment) error {
	if payment.PaymentMethod != nil && payment.PaymentMethod.Type == "" {
		return invalidPayment("payment method type is required")
	}

	if payment.Recipient != nil && payment.Recipient.AccountID == "" {
		return invalidPayment("payment recipient account_id is required")
	}

	if payment.IncomeAmount != nil {
		if payment.IncomeAmount.Value == "" || payment.IncomeAmount.Currency == "" {
			return invalidPayment("payment income amount requires value and currency")
		}
	}

	if payment.AuthorizationDetails != nil && payment.AuthorizationDetails.ThreeDSecure != nil {
		// ThreeDSecure is only valid inside authorization details; no extra fields to validate.
	}

	return nil
}

func translatePaymentError(err error) error {
	translated := sql.TranslateError(err)
	if translated != err {
		return translated
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if translated := translatePostgresCode(string(pqErr.Code)); translated != nil {
			return translated
		}
	}

	var pgxErr *pgconn.PgError
	if errors.As(err, &pgxErr) {
		if translated := translatePostgresCode(pgxErr.Code); translated != nil {
			return translated
		}
	}

	if strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		return invalidPayment("payment references missing related record")
	}

	return translated
}

func translatePostgresCode(code string) error {
	switch code {
	case "23503", "23514":
		return invalidPayment("payment violates storage constraints")
	case "23505":
		return sql.ErrConflict
	default:
		return nil
	}
}

func MapPaymentToModel(payment entities.Payment) (models.Payment, error) {
	if err := validateStoredPayment(payment); err != nil {
		return models.Payment{}, fmt.Errorf("validate payment snapshot %s: %w", payment.ID, err)
	}

	authorizationDetails, err := marshalAuthorizationDetails(payment.AuthorizationDetails)
	if err != nil {
		return models.Payment{}, err
	}

	metadata, err := marshalMetadata(payment.Metadata)
	if err != nil {
		return models.Payment{}, err
	}

	paymentMethod, err := marshalPaymentMethod(payment.PaymentMethod)
	if err != nil {
		return models.Payment{}, err
	}

	recipient, err := marshalRecipient(payment.Recipient)
	if err != nil {
		return models.Payment{}, err
	}

	model := models.Payment{
		ID:                   payment.ID,
		UserID:               payment.UserID,
		SubscriptionID:       payment.SubscriptionID,
		Purpose:              string(normalizePurpose(payment.Purpose)),
		Status:               string(payment.Status),
		Paid:                 payment.Paid,
		AmountValue:          payment.Amount.Value,
		AmountCurrency:       payment.Amount.Currency,
		Description:          payment.Description,
		ExpiresAt:            payment.ExpiresAt,
		Refundable:           payment.Refundable,
		Test:                 payment.Test,
		AuthorizationDetails: authorizationDetails,
		Metadata:             metadata,
		PaymentMethod:        paymentMethod,
		Recipient:            recipient,
		CreatedAt:            payment.CreatedAt,
	}

	if payment.IncomeAmount != nil {
		model.IncomeAmountValue = &payment.IncomeAmount.Value
		model.IncomeAmountCurrency = &payment.IncomeAmount.Currency
	}

	return model, nil
}

func invalidPayment(reason string) error {
	return fmt.Errorf("%s: %w", reason, sql.ErrInvalid)
}

func buildPaymentUpdateFields(payment entities.Payment) (map[string]interface{}, error) {
	authorizationDetails, err := marshalAuthorizationDetails(payment.AuthorizationDetails)
	if err != nil {
		return nil, err
	}

	metadata, err := marshalMetadata(payment.Metadata)
	if err != nil {
		return nil, err
	}

	paymentMethod, err := marshalPaymentMethod(payment.PaymentMethod)
	if err != nil {
		return nil, err
	}

	recipient, err := marshalRecipient(payment.Recipient)
	if err != nil {
		return nil, err
	}

	updateFields := map[string]interface{}{
		"subscription_id":        payment.SubscriptionID,
		"purpose":                string(normalizePurpose(payment.Purpose)),
		"status":                 string(payment.Status),
		"paid":                   payment.Paid,
		"description":            payment.Description,
		"expires_at":             payment.ExpiresAt,
		"refundable":             payment.Refundable,
		"test":                   payment.Test,
		"authorization_details":  authorizationDetails,
		"metadata":               metadata,
		"payment_method":         paymentMethod,
		"recipient":              recipient,
		"income_amount_value":    nil,
		"income_amount_currency": nil,
	}

	if payment.IncomeAmount != nil {
		updateFields["income_amount_value"] = payment.IncomeAmount.Value
		updateFields["income_amount_currency"] = payment.IncomeAmount.Currency
	}

	return updateFields, nil
}

func mapModelToPayment(paymentModel models.Payment) (entities.Payment, error) {
	authorizationDetails, err := unmarshalAuthorizationDetails(paymentModel.AuthorizationDetails)
	if err != nil {
		return entities.Payment{}, err
	}

	metadata, err := unmarshalMetadata(paymentModel.Metadata)
	if err != nil {
		return entities.Payment{}, err
	}

	paymentMethod, err := unmarshalPaymentMethod(paymentModel.PaymentMethod)
	if err != nil {
		return entities.Payment{}, err
	}

	recipient, err := unmarshalRecipient(paymentModel.Recipient)
	if err != nil {
		return entities.Payment{}, err
	}

	payment := entities.Payment{
		ID:             paymentModel.ID,
		UserID:         paymentModel.UserID,
		SubscriptionID: paymentModel.SubscriptionID,
		Purpose:        normalizePurpose(entities.PaymentPurpose(paymentModel.Purpose)),
		Status:         entities.Status(paymentModel.Status),
		Paid:           paymentModel.Paid,
		Amount: entities.Amount{
			Value:    paymentModel.AmountValue,
			Currency: paymentModel.AmountCurrency,
		},
		AuthorizationDetails: authorizationDetails,
		CreatedAt:            paymentModel.CreatedAt,
		Description:          paymentModel.Description,
		ExpiresAt:            paymentModel.ExpiresAt,
		Metadata:             metadata,
		PaymentMethod:        paymentMethod,
		Recipient:            recipient,
		Refundable:           paymentModel.Refundable,
		Test:                 paymentModel.Test,
	}

	if paymentModel.IncomeAmountValue != nil || paymentModel.IncomeAmountCurrency != nil {
		incomeAmount := entities.Amount{}
		if paymentModel.IncomeAmountValue != nil {
			incomeAmount.Value = *paymentModel.IncomeAmountValue
		}
		if paymentModel.IncomeAmountCurrency != nil {
			incomeAmount.Currency = *paymentModel.IncomeAmountCurrency
		}
		payment.IncomeAmount = &incomeAmount
	}

	return payment, nil
}

func normalizePurpose(purpose entities.PaymentPurpose) entities.PaymentPurpose {
	if purpose == "" {
		return entities.PaymentPurposeTopUp
	}

	return purpose
}

func marshalAuthorizationDetails(value *entities.AuthorizationDetails) (datatypes.JSON, error) {
	return marshalJSON(value)
}

func marshalMetadata(value interface{}) (datatypes.JSON, error) {
	return marshalJSON(value)
}

func marshalPaymentMethod(value *entities.PaymentMethodDetails) (datatypes.JSON, error) {
	return marshalJSON(value)
}

func marshalRecipient(value *entities.Recipient) (datatypes.JSON, error) {
	return marshalJSON(value)
}

func unmarshalAuthorizationDetails(raw datatypes.JSON) (*entities.AuthorizationDetails, error) {
	value, err := unmarshalJSON[entities.AuthorizationDetails](raw)
	if err != nil || value == nil {
		return value, err
	}

	return value, nil
}

func unmarshalMetadata(raw datatypes.JSON) (interface{}, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return value, nil
}

func unmarshalPaymentMethod(raw datatypes.JSON) (*entities.PaymentMethodDetails, error) {
	return unmarshalJSON[entities.PaymentMethodDetails](raw)
}

func unmarshalRecipient(raw datatypes.JSON) (*entities.Recipient, error) {
	return unmarshalJSON[entities.Recipient](raw)
}

func marshalJSON(value interface{}) (datatypes.JSON, error) {
	if value == nil {
		return nil, nil
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	return raw, nil
}

func unmarshalJSON[T any](raw datatypes.JSON) (*T, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}

	return &value, nil
}
