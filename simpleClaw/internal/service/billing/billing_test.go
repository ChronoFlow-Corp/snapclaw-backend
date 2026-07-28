package billing

import (
	"testing"

	"github.com/google/uuid"

	"simpleClaw/internal/entities"
)

func TestBillingEntitiesCompile(t *testing.T) {
	t.Parallel()

	plan := entities.Plan{
		ID:                 uuid.New(),
		Code:               "starter",
		Name:               "Starter",
		BillingAmountMinor: 99000,
		BalanceCreditMinor: 150000,
		Currency:           entities.RUB,
		IsActive:           true,
	}

	subscription := entities.UserSubscription{
		ID:     uuid.New(),
		UserID: uuid.New(),
		PlanID: plan.ID,
		Status: entities.SubscriptionStatusActive,
	}

	entry := entities.UserBalanceEntry{
		ID:          uuid.New(),
		UserID:      subscription.UserID,
		Type:        entities.BalanceEntryTypeSubscriptionCredit,
		AmountMinor: plan.BalanceCreditMinor,
	}

	if entry.AmountMinor <= 0 {
		t.Fatal("expected positive amount")
	}
}
