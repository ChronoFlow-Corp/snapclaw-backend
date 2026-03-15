package user

import (
	"context"
	"errors"
	"fmt"
	"shared/pkg/jwt"

	"simpleClaw/internal/infra/sql"

	"simpleClaw/internal/entities"

	"simpleClaw/internal/service/user/commands"

	"github.com/google/uuid"
)

type Service struct {
	j    jwt.JWT
	uSt  UStorage
	chSt ChannelStorage
	keys apiKeyManager
}

func NewUser(j jwt.JWT, uSt UStorage, chSt ChannelStorage, keys apiKeyManager) *Service {
	return &Service{
		j:    j,
		uSt:  uSt,
		chSt: chSt,
		keys: keys,
	}
}

func (s *Service) SignIn(
	ctx context.Context,
	cm commands.SignIn,
) (access jwt.AccessToken, refresh jwt.RefreshToken, err error) {
	const op = "service.Service.Sign"

	u, err := s.uSt.GetByEmail(ctx, cm.Email)
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	if errors.Is(err, sql.ErrNotFound) {
		u = entities.NewUser(cm.Name, cm.NickName, cm.AvatarURL, cm.Email, entities.UserRole)

		key, keyErr := s.keys.Create(ctx, u.ID, u.Name, 0)
		if keyErr != nil {
			return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, keyErr)
		}

		u.OpenRouterApiKey = key.Secret
		u.OpenRouterKeyID = key.ID

		err = s.uSt.Create(ctx, u)
		if err != nil {
			return jwt.AccessToken{}, jwt.RefreshToken{}, fmt.Errorf("%s: %w", op, err)
		}
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

	err := s.chSt.Create(ctx, ch)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, err)
	}

	return ch, nil
}

func (s *Service) RemoveChannel(ctx context.Context, cm commands.RemoveChannel) error {
	const op = "service.Service.RemoveChannel"

	err := s.chSt.Delete(ctx, cm.ChannelID, cm.UserID)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *Service) UpdateChannel(ctx context.Context, cm commands.UpdateChannel) error {
	const op = "service.Service.UpdateChannel"

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

	err := s.chSt.Update(ctx, ch)
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

	ch, err := s.chSt.GetByID(ctx, channelID, userID)
	if err != nil {
		return entities.Channel{}, fmt.Errorf("%s: %w", op, err)
	}

	return ch, nil
}

func (s *Service) GetChannels(ctx context.Context, userID uuid.UUID) ([]entities.Channel, error) {
	const op = "service.Service.GetChannels"

	chs, err := s.chSt.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return chs, nil
}
