package billing

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/sql"
	billingresult "simpleClaw/internal/service/billing/result"
)

type fakeBootstrapClawReader struct {
	clawsByUser map[uuid.UUID][]entities.Claw
}

func (r *fakeBootstrapClawReader) GetByUserID(_ context.Context, userID uuid.UUID) ([]entities.Claw, error) {
	return append([]entities.Claw(nil), r.clawsByUser[userID]...), nil
}

type fakeBootstrapManagedBotReader struct {
	botsByClaw map[uuid.UUID]entities.TelegramManagedBot
}

func (r *fakeBootstrapManagedBotReader) GetLatestManagedBotByClawID(
	_ context.Context,
	_ uuid.UUID,
	clawID uuid.UUID,
) (entities.TelegramManagedBot, error) {
	bot, ok := r.botsByClaw[clawID]
	if !ok {
		return entities.TelegramManagedBot{}, sql.ErrNotFound
	}

	return bot, nil
}

func TestGetBootstrap_DashboardAllowedBySubscriptionStatus(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	userID := uuid.New()
	plan := newActivePlan()

	testCases := []struct {
		name         string
		subscription *entities.UserSubscription
		wantAllowed  bool
		wantStep     string
	}{
		{
			name: "active subscription is allowed",
			subscription: &entities.UserSubscription{
				ID:                 uuid.New(),
				UserID:             userID,
				PlanID:             plan.ID,
				Status:             entities.SubscriptionStatusActive,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			wantAllowed: true,
			wantStep:    billingresult.OnboardingStepSubscriptionRequired,
		},
		{
			name: "canceled subscription remains allowed until period end",
			subscription: &entities.UserSubscription{
				ID:                 uuid.New(),
				UserID:             userID,
				PlanID:             plan.ID,
				Status:             entities.SubscriptionStatusCanceled,
				CurrentPeriodStart: now.Add(-48 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			wantAllowed: true,
			wantStep:    billingresult.OnboardingStepSubscriptionRequired,
		},
		{
			name: "canceled subscription loses access after period end",
			subscription: &entities.UserSubscription{
				ID:                 uuid.New(),
				UserID:             userID,
				PlanID:             plan.ID,
				Status:             entities.SubscriptionStatusCanceled,
				CurrentPeriodStart: now.Add(-72 * time.Hour),
				CurrentPeriodEnd:   now.Add(-time.Hour),
			},
			wantAllowed: false,
			wantStep:    billingresult.OnboardingStepSubscriptionRequired,
		},
		{
			name: "pending subscription is not allowed",
			subscription: &entities.UserSubscription{
				ID:                 uuid.New(),
				UserID:             userID,
				PlanID:             plan.ID,
				Status:             entities.SubscriptionStatusPending,
				CurrentPeriodStart: now,
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			wantAllowed: false,
			wantStep:    billingresult.OnboardingStepSubscriptionRequired,
		},
		{
			name: "past due subscription is not allowed",
			subscription: &entities.UserSubscription{
				ID:                 uuid.New(),
				UserID:             userID,
				PlanID:             plan.ID,
				Status:             entities.SubscriptionStatusPastDue,
				CurrentPeriodStart: now.Add(-24 * time.Hour),
				CurrentPeriodEnd:   now.Add(24 * time.Hour),
			},
			wantAllowed: false,
			wantStep:    billingresult.OnboardingStepSubscriptionRequired,
		},
		{
			name:         "missing subscription is not allowed",
			subscription: nil,
			wantAllowed:  false,
			wantStep:     billingresult.OnboardingStepSubscriptionRequired,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			subscriptions := newFakeSubscriptionStorage()
			if tc.subscription != nil {
				subscriptions.byID[tc.subscription.ID] = *tc.subscription
				subscriptions.byUser[userID] = *tc.subscription
			}

			service := newTestService(
				&fakePlanStorage{plans: map[uuid.UUID]entities.Plan{plan.ID: plan}},
				subscriptions,
				nil,
				&fakeUserStorage{users: map[uuid.UUID]entities.User{
					userID: {ID: userID},
				}},
				nil,
				nil,
			).WithBootstrapClawReader(&fakeBootstrapClawReader{})

			bootstrap, err := service.GetBootstrap(context.Background(), userID)
			if err != nil {
				t.Fatalf("GetBootstrap() error = %v", err)
			}

			if bootstrap.DashboardAllowed != tc.wantAllowed {
				t.Fatalf("DashboardAllowed = %v, want %v", bootstrap.DashboardAllowed, tc.wantAllowed)
			}

			if bootstrap.Onboarding.Step != tc.wantStep {
				t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, tc.wantStep)
			}
		})
	}
}

func TestGetBootstrap_MapsOnboardingResumeState(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	userID := uuid.New()
	plan := newActivePlan()
	baseService := func(subscription entities.UserSubscription, claws []entities.Claw) *Service {
		subscriptions := newFakeSubscriptionStorage()
		subscriptions.byID[subscription.ID] = subscription
		subscriptions.byUser[userID] = subscription

		return newTestService(
			&fakePlanStorage{plans: map[uuid.UUID]entities.Plan{plan.ID: plan}},
			subscriptions,
			nil,
			&fakeUserStorage{users: map[uuid.UUID]entities.User{
				userID: {ID: userID},
			}},
			nil,
			nil,
		).WithBootstrapClawReader(&fakeBootstrapClawReader{
			clawsByUser: map[uuid.UUID][]entities.Claw{userID: claws},
		}).WithBootstrapManagedBotReader(&fakeBootstrapManagedBotReader{})
	}

	activeSubscription := entities.UserSubscription{
		ID:                 uuid.New(),
		UserID:             userID,
		PlanID:             plan.ID,
		Status:             entities.SubscriptionStatusActive,
		CurrentPeriodStart: now.Add(-24 * time.Hour),
		CurrentPeriodEnd:   now.Add(24 * time.Hour),
	}

	t.Run("no claw resets onboarding", func(t *testing.T) {
		bootstrap, err := baseService(activeSubscription, nil).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepSubscriptionRequired {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepSubscriptionRequired)
		}
		if bootstrap.Onboarding.ClawID != nil {
			t.Fatalf("Onboarding.ClawID = %v, want nil", bootstrap.Onboarding.ClawID)
		}
	})

	t.Run("claw without telegram binding resumes at telegram choice", func(t *testing.T) {
		clawID := uuid.New()
		bootstrap, err := baseService(activeSubscription, []entities.Claw{{
			ID:                 clawID,
			UserID:             userID,
			CreatedAt:          now.Add(-time.Hour),
			UpdatedAt:          now,
			OnboardingComplete: false,
		}}).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramChoice {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramChoice)
		}
		if bootstrap.Onboarding.ClawID == nil || *bootstrap.Onboarding.ClawID != clawID {
			t.Fatalf("Onboarding.ClawID = %v, want %s", bootstrap.Onboarding.ClawID, clawID)
		}
	})

	t.Run("canceled subscription with remaining access resumes at telegram choice", func(t *testing.T) {
		clawID := uuid.New()
		subscription := entities.UserSubscription{
			ID:                 uuid.New(),
			UserID:             userID,
			PlanID:             plan.ID,
			Status:             entities.SubscriptionStatusCanceled,
			CurrentPeriodStart: now.Add(-48 * time.Hour),
			CurrentPeriodEnd:   now.Add(24 * time.Hour),
		}

		bootstrap, err := baseService(subscription, []entities.Claw{{
			ID:                 clawID,
			UserID:             userID,
			CreatedAt:          now.Add(-time.Hour),
			UpdatedAt:          now,
			OnboardingComplete: false,
		}}).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if !bootstrap.DashboardAllowed {
			t.Fatalf("DashboardAllowed = false, want true")
		}
		if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramChoice {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramChoice)
		}
		if bootstrap.Onboarding.ClawID == nil || *bootstrap.Onboarding.ClawID != clawID {
			t.Fatalf("Onboarding.ClawID = %v, want %s", bootstrap.Onboarding.ClawID, clawID)
		}
	})

	t.Run("expired canceled subscription returns to subscription required", func(t *testing.T) {
		clawID := uuid.New()
		subscription := entities.UserSubscription{
			ID:                 uuid.New(),
			UserID:             userID,
			PlanID:             plan.ID,
			Status:             entities.SubscriptionStatusCanceled,
			CurrentPeriodStart: now.Add(-72 * time.Hour),
			CurrentPeriodEnd:   now,
		}

		bootstrap, err := baseService(subscription, []entities.Claw{{
			ID:                 clawID,
			UserID:             userID,
			CreatedAt:          now.Add(-time.Hour),
			UpdatedAt:          now,
			OnboardingComplete: false,
		}}).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.DashboardAllowed {
			t.Fatalf("DashboardAllowed = true, want false")
		}
		if bootstrap.Onboarding.Step != billingresult.OnboardingStepSubscriptionRequired {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepSubscriptionRequired)
		}
		if bootstrap.Onboarding.ClawID == nil || *bootstrap.Onboarding.ClawID != clawID {
			t.Fatalf("Onboarding.ClawID = %v, want %s", bootstrap.Onboarding.ClawID, clawID)
		}
	})

	t.Run("claw waiting for telegram approval resumes at confirm step", func(t *testing.T) {
		bootstrap, err := baseService(activeSubscription, []entities.Claw{{
			ID:     uuid.New(),
			UserID: userID,
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &entitychannels.TelegramConfig{
						Enabled:  true,
						DmPolicy: entitychannels.DmPairing,
					},
				},
			},
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		}}).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramConfirm {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramConfirm)
		}
	})

	t.Run("completed onboarding returns dashboard ready", func(t *testing.T) {
		bootstrap, err := baseService(activeSubscription, []entities.Claw{{
			ID:                 uuid.New(),
			UserID:             userID,
			OnboardingComplete: true,
			CreatedAt:          now.Add(-time.Hour),
			UpdatedAt:          now,
		}}).GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Required {
			t.Fatalf("Onboarding.Required = true, want false")
		}
		if bootstrap.Onboarding.Step != billingresult.OnboardingStepDashboardReady {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepDashboardReady)
		}
	})

	t.Run("managed bot pending link resumes at manager link step", func(t *testing.T) {
		clawID := uuid.New()
		managedBotID := uuid.New()
		service := baseService(activeSubscription, []entities.Claw{{
			ID:        clawID,
			UserID:    userID,
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		}}).WithBootstrapManagedBotReader(&fakeBootstrapManagedBotReader{
			botsByClaw: map[uuid.UUID]entities.TelegramManagedBot{
				clawID: {
					ID:            managedBotID,
					UserID:        userID,
					ClawID:        clawID,
					Status:        entities.TelegramManagedBotStatusPendingLink,
					DeepLinkURL:   "https://t.me/simpleclaw_manager_bot?start=resume-code",
					LinkExpiresAt: now.Add(10 * time.Minute),
				},
			},
		})

		bootstrap, err := service.GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramManagerLink {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramManagerLink)
		}

		if bootstrap.Onboarding.TelegramManager == nil || bootstrap.Onboarding.TelegramManager.ID != managedBotID {
			t.Fatalf("Onboarding.TelegramManager = %#v", bootstrap.Onboarding.TelegramManager)
		}
	})

	t.Run("managed bot provisioning resumes while linked or waiting creation", func(t *testing.T) {
		for _, status := range []entities.TelegramManagedBotStatus{
			entities.TelegramManagedBotStatusLinked,
			entities.TelegramManagedBotStatusWaitingCreation,
		} {
			status := status
			t.Run(string(status), func(t *testing.T) {
				clawID := uuid.New()
				service := baseService(activeSubscription, []entities.Claw{{
					ID:        clawID,
					UserID:    userID,
					CreatedAt: now.Add(-time.Hour),
					UpdatedAt: now,
				}}).WithBootstrapManagedBotReader(&fakeBootstrapManagedBotReader{
					botsByClaw: map[uuid.UUID]entities.TelegramManagedBot{
						clawID: {
							ID:     uuid.New(),
							UserID: userID,
							ClawID: clawID,
							Status: status,
						},
					},
				})

				bootstrap, err := service.GetBootstrap(context.Background(), userID)
				if err != nil {
					t.Fatalf("GetBootstrap() error = %v", err)
				}

				if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramManagerProvisioning {
					t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramManagerProvisioning)
				}
			})
		}
	})

	t.Run("managed bot failure falls back to manual connect", func(t *testing.T) {
		clawID := uuid.New()
		service := baseService(activeSubscription, []entities.Claw{{
			ID:        clawID,
			UserID:    userID,
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		}}).WithBootstrapManagedBotReader(&fakeBootstrapManagedBotReader{
			botsByClaw: map[uuid.UUID]entities.TelegramManagedBot{
				clawID: {
					ID:        uuid.New(),
					UserID:    userID,
					ClawID:    clawID,
					Status:    entities.TelegramManagedBotStatusFailed,
					LastError: "telegram bot provisioning failed",
				},
			},
		})

		bootstrap, err := service.GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepTelegramManualConnect {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepTelegramManualConnect)
		}

		if bootstrap.Onboarding.TelegramManager == nil || bootstrap.Onboarding.TelegramManager.LastError != "telegram bot provisioning failed" {
			t.Fatalf("Onboarding.TelegramManager = %#v", bootstrap.Onboarding.TelegramManager)
		}
	})

	t.Run("managed bot ready unlocks dashboard", func(t *testing.T) {
		clawID := uuid.New()
		service := baseService(activeSubscription, []entities.Claw{{
			ID:        clawID,
			UserID:    userID,
			CreatedAt: now.Add(-time.Hour),
			UpdatedAt: now,
		}}).WithBootstrapManagedBotReader(&fakeBootstrapManagedBotReader{
			botsByClaw: map[uuid.UUID]entities.TelegramManagedBot{
				clawID: {
					ID:     uuid.New(),
					UserID: userID,
					ClawID: clawID,
					Status: entities.TelegramManagedBotStatusReady,
				},
			},
		})

		bootstrap, err := service.GetBootstrap(context.Background(), userID)
		if err != nil {
			t.Fatalf("GetBootstrap() error = %v", err)
		}

		if bootstrap.Onboarding.Required {
			t.Fatalf("Onboarding.Required = true, want false")
		}

		if bootstrap.Onboarding.Step != billingresult.OnboardingStepDashboardReady {
			t.Fatalf("Onboarding.Step = %q, want %q", bootstrap.Onboarding.Step, billingresult.OnboardingStepDashboardReady)
		}
	})
}
