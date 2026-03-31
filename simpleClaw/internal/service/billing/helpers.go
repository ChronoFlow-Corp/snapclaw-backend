package billing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func applyPaymentEventSnapshot(
	current entities.Payment,
	event entities.Payment,
) entities.Payment {
	current.ID = event.ID
	current.Status = event.Status
	current.Paid = event.Paid
	current.Amount = event.Amount
	current.AuthorizationDetails = event.AuthorizationDetails
	current.CreatedAt = event.CreatedAt
	current.Description = event.Description
	current.Confirmation = event.Confirmation
	current.ExpiresAt = event.ExpiresAt
	current.Metadata = event.Metadata
	current.PaymentMethod = event.PaymentMethod
	current.Recipient = event.Recipient
	current.Refundable = event.Refundable
	current.Test = event.Test
	current.IncomeAmount = event.IncomeAmount

	return current
}

func (s *Service) alreadyProcessed(
	ctx context.Context,
	userID uuid.UUID,
	paymentID, entryType string,
) (bool, error) {
	entries, err := s.balance.ListByUserID(ctx, userID, 100)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.PaymentID == nil {
			continue
		}

		if *entry.PaymentID == paymentID && entry.Type == entryType {
			return true, nil
		}
	}

	return false, nil
}

func validatePlan(plan entities.Plan) error {
	switch {
	case plan.ID == uuid.Nil:
		return sql.ErrInvalid
	case plan.Code == "":
		return sql.ErrInvalid
	case plan.Name == "":
		return sql.ErrInvalid
	case plan.BillingAmountMinor <= 0:
		return sql.ErrInvalid
	case plan.BalanceCreditMinor <= 0:
		return sql.ErrInvalid
	case plan.Currency == "":
		return sql.ErrInvalid
	default:
		return nil
	}
}

func validateSuccessfulPayment(
	payment entities.Payment,
	wantPurpose entities.PaymentPurpose,
) error {
	switch {
	case payment.ID == "":
		return sql.ErrInvalid
	case payment.UserID == uuid.Nil:
		return sql.ErrInvalid
	case !payment.Paid:
		return decorateValidation(ErrPaymentNotPaid)
	case payment.CreatedAt.IsZero():
		return sql.ErrInvalid
	case normalizePurpose(payment.Purpose) != wantPurpose:
		return sql.ErrInvalid
	default:
		return nil
	}
}

func normalizePurpose(purpose entities.PaymentPurpose) entities.PaymentPurpose {
	if purpose == "" {
		return entities.PaymentPurposeTopUp
	}

	return purpose
}

func isInsufficientBalanceErr(err error) bool {
	return errors.Is(err, ErrInsufficientBalance) ||
		strings.Contains(strings.ToLower(err.Error()), "insufficient balance")
}

func parseUserIDFromOpenRouterAPIKeyName(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, decorateValidation(ErrOpenRouterAPIKeyInvalid)
	}

	idx := strings.Split(raw, "claw-")
	if len(idx) != 2 {
		return uuid.Nil, decorateValidation(ErrOpenRouterAPIKeyInvalid)
	}

	userID, err := uuid.Parse(idx[1])
	if err != nil {
		return uuid.Nil, decorateValidation(ErrOpenRouterAPIKeyInvalid)
	}

	return userID, nil
}

func sanitizeAPIKeyName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	if len(raw) <= 48 {
		return raw
	}

	return raw[:24] + "..." + raw[len(raw)-12:]
}

type defaultUsageAmountConverter struct{}

func (defaultUsageAmountConverter) ToMinor(
	totalCost string,
	sourceCurrency, targetCurrency string,
) (int64, error) {
	if strings.TrimSpace(sourceCurrency) != defaultOpenRouterCostCurrency ||
		strings.TrimSpace(targetCurrency) != defaultOpenRouterCostCurrency {
		return 0, decorateValidation(ErrUnsupportedCurrency)
	}

	parsed, err := strconv.ParseFloat(strings.TrimSpace(totalCost), 64)
	if err != nil {
		return 0, sql.ErrInvalid
	}

	if parsed < 0 {
		return 0, sql.ErrInvalid
	}

	return int64(math.Round(parsed * 100)), nil
}

func devModelsCosts(inputTokens, outputTokens int) {
	type models struct {
		Name       string
		InputCost  float64
		OutputCost float64
	}

	m := []models{
		{
			Name:       "Claude opus 4.6",
			InputCost:  5,
			OutputCost: 25,
		},
		{
			Name:       "GPT 5.4",
			InputCost:  2.50,
			OutputCost: 15,
		},
		{
			Name:       "Google: Gemini 3 Flash Preview",
			InputCost:  0.50,
			OutputCost: 3,
		},
		{
			Name:       "Qwen3.5 Flash",
			InputCost:  0.065,
			OutputCost: 0.26,
		},
		{
			Name:       "Kimi K2.5",
			InputCost:  0.42,
			OutputCost: 2.20,
		},
		{
			Name:       "GLM 5",
			InputCost:  1.20,
			OutputCost: 4,
		},
	}

	for _, model := range m {
		modelCost(model.Name, model.InputCost, model.OutputCost, inputTokens, outputTokens)
	}
}

func modelCost(model string, inputCost, outputCost float64, inputTokens, outputTokens int) {
	inputCostD := decimal.NewFromFloat(inputCost)
	outputCostD := decimal.NewFromFloat(outputCost)
	inputD := decimal.NewFromInt(int64(inputTokens))
	outputD := decimal.NewFromInt(int64(outputTokens))

	claudeCostPerTokenInput := inputCostD.Div(decimal.NewFromInt(1_000_000))
	claudeCostPerTokenOutput := outputCostD.Div(decimal.NewFromInt(1_000_000))

	fmt.Println(fmt.Sprintf(
		"%s TOTAL COST: %s$",
		model,
		claudeCostPerTokenInput.Mul(inputD).Add(claudeCostPerTokenOutput.Mul(outputD)).String(),
	))
}
