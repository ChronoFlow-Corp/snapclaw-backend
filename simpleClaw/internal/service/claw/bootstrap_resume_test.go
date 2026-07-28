package claw

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	"simpleClaw/internal/service/claw/commands"
)

func TestServiceStart_CompletesOnboardingWhenApproveStepIsNotRequired(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:                 clawID,
			UserID:             userID,
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}
	operations := &updateTestOperationStorage{}

	service := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{},
		"",
		GmailWatchConfig{},
	)

	_, err := service.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if storage.lifecycleUpdate.OnboardingComplete == nil || !*storage.lifecycleUpdate.OnboardingComplete {
		t.Fatalf("OnboardingComplete = %v, want true", storage.lifecycleUpdate.OnboardingComplete)
	}
}

func TestServiceStart_KeepsOnboardingIncompleteWhenTelegramApproveIsRequired(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:     clawID,
			UserID: userID,
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &entitychannels.TelegramConfig{
						Enabled:  true,
						DmPolicy: entitychannels.DmPairing,
					},
				},
			},
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}
	operations := &updateTestOperationStorage{}

	service := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{},
		"",
		GmailWatchConfig{},
	)

	_, err := service.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if storage.lifecycleUpdate.OnboardingComplete == nil {
		t.Fatal("OnboardingComplete = nil, want false pointer")
	}
	if *storage.lifecycleUpdate.OnboardingComplete {
		t.Fatalf("OnboardingComplete = true, want false")
	}
}

func TestServiceStart_PreservesOnboardingCompleteForApprovedClaw(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:                 clawID,
			UserID:             userID,
			OnboardingComplete: true,
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &entitychannels.TelegramConfig{
						Enabled:  true,
						DmPolicy: entitychannels.DmPairing,
					},
				},
			},
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}
	operations := &updateTestOperationStorage{}

	service := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{},
		"",
		GmailWatchConfig{},
	)

	_, err := service.Start(context.Background(), commands.StartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if storage.lifecycleUpdate.OnboardingComplete == nil || !*storage.lifecycleUpdate.OnboardingComplete {
		t.Fatalf("OnboardingComplete = %v, want true", storage.lifecycleUpdate.OnboardingComplete)
	}
}

func TestServiceRestart_PreservesOnboardingCompleteForApprovedClaw(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	storage := &updateTestClawStorage{
		existing: entities.Claw{
			ID:                 clawID,
			UserID:             userID,
			OnboardingComplete: true,
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &entitychannels.TelegramConfig{
						Enabled:  true,
						DmPolicy: entitychannels.DmPairing,
					},
				},
			},
			ClawLifecycleState: entities.NewClawLifecycleState(),
		},
	}
	operations := &updateTestOperationStorage{}

	service := NewClaw(
		storage,
		operations,
		&updateTestChannelStorage{},
		&updateTestUserStorage{},
		&updateTestServerStorage{},
		&updateTestHosting{},
		&updateTestKeys{},
		"",
		GmailWatchConfig{},
	)

	_, err := service.Restart(context.Background(), commands.RestartClaw{
		UserID: userID,
		ClawID: clawID,
	})
	if err != nil {
		t.Fatalf("Restart() error = %v", err)
	}

	if storage.lifecycleUpdate.OnboardingComplete == nil || !*storage.lifecycleUpdate.OnboardingComplete {
		t.Fatalf("OnboardingComplete = %v, want true", storage.lifecycleUpdate.OnboardingComplete)
	}
}

func TestServiceApprovePairing_CompletesOnboarding(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	clawID := uuid.New()
	serverID := uuid.New()
	hostingStub := &approveTestHosting{}
	storage := &approveTestClawStorage{
		claw: entities.Claw{
			ID:          clawID,
			UserID:      userID,
			ServerID:    serverID,
			ContainerID: "container-1",
			Config: entities.ClawConfig{
				Channels: &entities.ClawChannels{
					Telegram: &entitychannels.TelegramConfig{
						Enabled:  true,
						DmPolicy: entitychannels.DmPairing,
					},
				},
			},
		},
	}

	service := NewClaw(
		storage,
		&approveTestOperationStorage{},
		&approveTestChannelStorage{},
		&approveTestUserStorage{},
		&approveTestServerStorage{server: entities.Server{ID: serverID, URL: "http://runtime.local", SecretKey: "secret"}},
		hostingStub,
		&approveTestKeys{},
		"",
		GmailWatchConfig{},
	)

	err := service.ApprovePairing(context.Background(), commands.ApprovePairing{
		UserID: userID,
		ClawID: clawID,
		Code:   "123456",
	})
	if err != nil {
		t.Fatalf("ApprovePairing() error = %v", err)
	}

	if storage.lifecycleUpdate.OnboardingComplete == nil || !*storage.lifecycleUpdate.OnboardingComplete {
		t.Fatalf("OnboardingComplete = %v, want true", storage.lifecycleUpdate.OnboardingComplete)
	}
}
