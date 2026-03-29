package user

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"shared/consts"
	"shared/pkg/jwt"
	"shared/pkg/observability"
	"strings"
	"time"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/user/commands"

	"github.com/google/uuid"
)

type Service struct {
	j       jwt.JWT
	uSt     UStorage
	chSt    ChannelStorage
	pmSt    paymentMethodStorage
	keys    apiKeyManager
	admins  map[string]struct{}
	metrics *observability.OperationMetrics
}

func NewUser(
	j jwt.JWT,
	uSt UStorage,
	chSt ChannelStorage,
	pmSt paymentMethodStorage,
	keys apiKeyManager,
	admins []string,
	metrics ...*observability.OperationMetrics,
) *Service {
	var opMetrics *observability.OperationMetrics

	if len(metrics) > 0 {
		opMetrics = metrics[0]
	}

	return &Service{
		j:       j,
		uSt:     uSt,
		chSt:    chSt,
		pmSt:    pmSt,
		keys:    keys,
		admins:  buildAdminSet(admins),
		metrics: opMetrics,
	}
}

func (s *Service) SignIn(
	ctx context.Context,
	cm commands.SignIn,
) (access jwt.AccessToken, refresh jwt.RefreshToken, err error) {
	const op = "service.Service.Sign"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"auth.sign_in",
		"auth",
	)

	defer func() { finish(err) }()

	email := normalizeEmail(cm.Email)
	role := s.roleForEmail(email)

	u, err := s.uSt.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	if errors.Is(err, sql.ErrNotFound) {
		u = entities.NewUser(cm.Name, cm.NickName, cm.AvatarURL, email, role)

		if s.keys != nil {
			key, keyErr := s.keys.Create(ctx, u.ID, u.Name, 0)
			if keyErr != nil {
				return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, keyErr)
			}

			u.OpenRouterApiKey = key.Secret
			u.OpenRouterKeyID = key.ID
		}

		err = s.uSt.Create(ctx, u)
		if err != nil {
			return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
		}
	} else if u.Role != role {
		err := s.uSt.UpdateRole(ctx, u.ID, role)
		if err != nil {
			return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
		}

		u.Role = role
	}

	session := entities.NewSession(u.ID)

	access, refresh, err = s.j.GeneratePair(u.ID, session.ID)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	session.RefreshToken = refresh.Raw

	err = s.uSt.CreateSession(ctx, session)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return access, refresh, nil
}

func (s *Service) SignOut() {
	const op = "service.Service.SignOut"
}

func (s *Service) Refresh(
	ctx context.Context,
	rawRefresh string,
) (access jwt.AccessToken, refresh jwt.RefreshToken, err error) {
	const op = "service.Service.Refresh"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"auth.refresh",
		"auth",
	)

	defer func() { finish(err) }()

	t, err := s.j.ParseRefresh(rawRefresh)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	session, err := s.uSt.GetSession(ctx, t.Claims.SessionID)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	if session.UserID != t.Claims.UserID {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, sql.ErrNotFound)
	}

	if session.RefreshToken == "" || session.RefreshToken != rawRefresh {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, jwt.ErrInvalid)
	}

	access, refresh, err = s.j.GeneratePair(t.Claims.UserID, t.Claims.SessionID)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	err = s.uSt.UpdateSessionRefresh(ctx, session.ID, session.UserID, refresh.Raw)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return access, refresh, nil
}

func (s *Service) UserInfo(ctx context.Context, userID uuid.UUID) (entities.User, error) {
	const op = "service.Service.UserInfo"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"user.info.get",
		"user_profile",
	)

	var err error

	defer func() { finish(err) }()

	user, err := s.uSt.GetByID(ctx, userID)
	if err != nil {
		return entities.User{}, fmt.Errorf("%s: %w", op, err)
	}

	return user, nil
}

func (s *Service) AddChannel(
	ctx context.Context,
	cm commands.AddChannel,
) (entities.Channel, error) {
	const op = "service.Service.AddChannel"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.add",
		"channel_connect",
	)

	var err error

	defer func() { finish(err) }()

	var ch entities.Channel

	switch {
	case cm.Telegram != nil:
		cfg := addTgCfg(*cm.Telegram)
		ch = entities.NewChannel(
			entities.ChannelTelegramType,
			cm.Name,
			entities.ClawChannels{Telegram: &cfg},
			cm.UserID,
		)
	default:
		return entities.Channel{}, ErrChannelUnsupported
	}

	err = s.chSt.Create(ctx, ch)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, err)
	}

	return ch, nil
}

func (s *Service) RemoveChannel(ctx context.Context, cm commands.RemoveChannel) (err error) {
	const op = "service.Service.RemoveChannel"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.delete",
		"channel_connect",
	)

	defer func() { finish(err) }()

	err = s.chSt.Delete(ctx, cm.ChannelID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) UpdateChannel(ctx context.Context, cm commands.UpdateChannel) (err error) {
	const op = "service.Service.UpdateChannel"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.update",
		"channel_connect",
	)

	defer func() { finish(err) }()

	var ch entities.Channel

	switch {
	case cm.Telegram != nil:
		cfg := addTgCfg(*cm.Telegram)
		ch = entities.NewChannel(
			entities.ChannelTelegramType,
			cm.Name,
			entities.ClawChannels{Telegram: &cfg},
			cm.UserID,
		)
	default:
		return ErrChannelUnsupported
	}

	ch.ID = cm.ChannelID

	err = s.chSt.Update(ctx, ch)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) GetChannel(
	ctx context.Context,
	channelID, userID uuid.UUID,
) (entities.Channel, error) {
	const op = "service.Service.GetChannel"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.get",
		"channel_connect",
	)

	var err error

	defer func() { finish(err) }()

	ch, err := s.chSt.GetByID(ctx, channelID, userID)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, err)
	}

	return ch, nil
}

func (s *Service) GetChannels(ctx context.Context, userID uuid.UUID) ([]entities.Channel, error) {
	const op = "service.Service.GetChannels"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.list",
		"channel_connect",
	)

	var err error

	defer func() { finish(err) }()

	chs, err := s.chSt.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return chs, nil
}

func (s *Service) Connect(ctx context.Context, cm commands.ConnectCommand) (err error) {
	const op = "service.Service.Connect"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"channel.connect",
		"channel_connect",
	)

	defer func() { finish(err) }()

	provider := strings.ToLower(strings.TrimSpace(cm.Provider))
	if provider != consts.ProviderGmail {
		return fmt.Errorf("%s: %w", op, ErrProviderUnsupported)
	}

	if cm.UserID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	if cm.AccessToken == "" {
		return fmt.Errorf("%s: %w", op, ErrAccessTokenRequired)
	}

	expiry := ""

	if !cm.ExpiresAt.IsZero() {
		expiry = cm.ExpiresAt.UTC().Format(time.RFC3339)
	}

	token := entities.GmailToken{
		Email:  strings.TrimSpace(cm.Email),
		Client: "default",
		Token: entities.Token{
			AccessToken:  cm.AccessToken,
			RefreshToken: cm.RefreshToken,
			TokenType:    "Bearer",
			Expiry:       expiry,
		},
	}

	existing, err := s.uSt.GetGmailToken(ctx, cm.UserID)
	switch {
	case err == nil:
		if token.Email == "" {
			token.Email = existing.Email
		}

		if token.Client == "" {
			token.Client = existing.Client
		}

		if token.Token.RefreshToken == "" {
			token.Token.RefreshToken = existing.Token.RefreshToken
		}

		if token.Token.TokenType == "" {
			token.Token.TokenType = existing.Token.TokenType
		}

		if token.Token.Expiry == "" {
			token.Token.Expiry = existing.Token.Expiry
		}
	case err != nil && !errors.Is(err, sql.ErrNotFound):
		return fmt.Errorf("%s: %w", op, err)
	}

	if token.Email == "" {
		user, err := s.uSt.GetByID(ctx, cm.UserID)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}

		token.Email = user.Email
	}

	if token.Client == "" {
		token.Client = "default"
	}

	if token.Token.TokenType == "" {
		token.Token.TokenType = "Bearer"
	}

	if err := s.uSt.UpsertGmailToken(ctx, cm.UserID, token); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) AddPaymentMethod(
	ctx context.Context,
	cm commands.AddPaymentMethod,
) (entities.PaymentMethod, error) {
	const op = "service.Service.AddPaymentMethod"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"payment_method.add",
		"payment_method",
	)

	var err error

	defer func() { finish(err) }()

	title := strings.TrimSpace(cm.Title)
	if cm.UserID == uuid.Nil || title == "" {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	count, err := s.pmSt.CountByUserID(ctx, cm.UserID)
	if err != nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, err)
	}

	isDefault := cm.IsDefault
	if count == 0 {
		isDefault = true
	} else if cm.IsDefault {
		if err = s.pmSt.ClearDefaultByUserID(ctx, cm.UserID); err != nil {
			return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	method := entities.PaymentMethod{
		ID:        uuid.New(),
		UserID:    cm.UserID,
		Title:     title,
		IsDefault: isDefault,
		CreatedAt: time.Now().UTC(),
	}

	if err = s.pmSt.Create(ctx, method); err != nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, err)
	}

	return method, nil
}

func (s *Service) GetPaymentMethod(
	ctx context.Context,
	methodID, userID uuid.UUID,
) (entities.PaymentMethod, error) {
	const op = "service.Service.GetPaymentMethod"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"payment_method.get",
		"payment_method",
	)

	var err error

	defer func() { finish(err) }()

	if methodID == uuid.Nil || userID == uuid.Nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	method, err := s.pmSt.GetByID(ctx, methodID, userID)
	if err != nil {
		return entities.PaymentMethod{}, fmt.Errorf("%s: %w", op, err)
	}

	return method, nil
}

func (s *Service) GetPaymentMethods(
	ctx context.Context,
	userID uuid.UUID,
) ([]entities.PaymentMethod, error) {
	const op = "service.Service.GetPaymentMethods"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"payment_method.list",
		"payment_method",
	)

	var err error

	defer func() { finish(err) }()

	if userID == uuid.Nil {
		return nil, fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	methods, err := s.pmSt.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return methods, nil
}

func (s *Service) SetDefaultPaymentMethod(
	ctx context.Context,
	cm commands.SetDefaultPaymentMethod,
) error {
	const op = "service.Service.SetDefaultPaymentMethod"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"payment_method.default.set",
		"payment_method",
	)

	var err error

	defer func() { finish(err) }()

	if cm.UserID == uuid.Nil || cm.PaymentMethodID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	if _, err = s.pmSt.GetByID(ctx, cm.PaymentMethodID, cm.UserID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err = s.pmSt.ClearDefaultByUserID(ctx, cm.UserID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if err = s.pmSt.UpdateDefault(ctx, cm.PaymentMethodID, cm.UserID, true); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) RemovePaymentMethod(
	ctx context.Context,
	methodID, userID uuid.UUID,
) error {
	const op = "service.Service.RemovePaymentMethod"

	ctx, _, finish := observability.StartOperation(
		ctx,
		slog.Default(),
		s.metrics,
		"service.user",
		"payment_method.delete",
		"payment_method",
	)

	var err error

	defer func() { finish(err) }()

	if methodID == uuid.Nil || userID == uuid.Nil {
		return fmt.Errorf("%s: %w", op, sql.ErrInvalid)
	}

	if err = s.pmSt.Delete(ctx, methodID, userID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}
