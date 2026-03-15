package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"
	"time"

	"simpleClaw/internal/api/rest/middleware"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"

	"simpleClaw/config"

	"simpleClaw/internal/service/user/commands"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/markbates/goth/gothic"
)

type service interface {
	SignIn(
		ctx context.Context,
		cm commands.SignIn,
	) (access jwt.AccessToken, refresh jwt.RefreshToken, err error)
	Refresh(
		ctx context.Context,
		rawRefresh string,
	) (access jwt.AccessToken, refresh jwt.RefreshToken, err error)
	UserInfo(ctx context.Context, userID uuid.UUID) (entities.User, error)

	AddChannel(
		ctx context.Context,
		cm commands.AddChannel,
	) (entities.Channel, error)
}
type User struct {
	env         string
	service     service
	frontendURL string
	j           jwt.JWT
}

func NewUser(env string, service service, j jwt.JWT, frontendURL string) *User {
	return &User{
		env:         env,
		service:     service,
		j:           j,
		frontendURL: frontendURL,
	}
}

func (u *User) Register(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Get("/connect/{provider}", u.Login)
		r.Get("/connect/{provider}/callback", u.Callback)
		r.Post("/refresh", u.Refresh)
	})

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthJwt(u.j))
		r.Get("/auth/user-info", u.UserInfo)
		r.Post("/channel", u.AddChannel)
	})
}

func (u *User) Login(w http.ResponseWriter, r *http.Request) {
	if gothUser, err := gothic.CompleteUserAuth(w, r); err == nil {
		access, refresh, err := u.service.SignIn(r.Context(), commands.SignIn{
			NickName:  gothUser.NickName,
			AvatarURL: gothUser.AvatarURL,
			Name:      gothUser.FirstName,
			Email:     gothUser.Email,
		})
		if err != nil {
			respondServiceError(w, err)
			return
		}

		u.setCookie(w, "/", "access_token", access.Raw, access.Claims.ExpiresAt.Time)
		u.setCookie(w, "/", "refresh_token", refresh.Raw, refresh.Claims.ExpiresAt.Time)
		http.Redirect(w, r, u.frontendURL, http.StatusFound)
	} else {
		gothic.BeginAuthHandler(w, r)
	}
}

func (u *User) Callback(w http.ResponseWriter, r *http.Request) {
	gothUser, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		return
	}

	access, refresh, err := u.service.SignIn(r.Context(), commands.SignIn{
		NickName:  gothUser.NickName,
		AvatarURL: gothUser.AvatarURL,
		Name:      gothUser.FirstName,
		Email:     gothUser.Email,
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	u.setCookie(w, "/", "access_token", access.Raw, access.Claims.ExpiresAt.Time)
	u.setCookie(w, "/", "refresh_token", refresh.Raw, refresh.Claims.ExpiresAt.Time)

	http.Redirect(w, r, u.frontendURL, http.StatusFound)
}

func (u *User) AddChannel(w http.ResponseWriter, r *http.Request) {
	var req dto.AddChannelRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: "invalid request body",
		})

		return
	}

	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	ch, err := u.service.AddChannel(r.Context(), commands.AddChannel{
		UserID: userID,
		Name:   req.Name,
		Telegram: &commands.TelegramChannel{
			DmPolicy:  req.TelegramChannel.DmPolicy,
			BotToken:  req.TelegramChannel.BotToken,
			AllowFrom: req.TelegramChannel.AllowFrom,
		},
	})
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, ch)
}

func (u *User) Refresh(w http.ResponseWriter, r *http.Request) {
	refreshCookie, err := r.Cookie("refresh_token")
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "Refresh token required",
		})
		return
	}

	if err = refreshCookie.Valid(); err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "Refresh token is invalid",
		})
		return
	}

	access, refresh, err := u.service.Refresh(r.Context(), refreshCookie.Value)
	if err != nil {
		code := http.StatusUnauthorized
		message := "Refresh token invalid"

		switch {
		case errors.Is(err, jwt.ErrExpired):
			message = "Refresh token expired"
		case errors.Is(err, jwt.ErrInvalid), errors.Is(err, sql.ErrNotFound):
			message = "Refresh token invalid"
		default:
			code = http.StatusInternalServerError
			message = "internal error"
		}

		response.RespondError(w, response.Error{Code: code, Message: message})

		return
	}

	u.setCookie(w, "/", "access_token", access.Raw, access.Claims.ExpiresAt.Time)
	u.setCookie(w, "/", "refresh_token", refresh.Raw, refresh.Claims.ExpiresAt.Time)

	response.RespondOK(w, map[string]any{"refreshed": true})
}

func (u *User) UserInfo(w http.ResponseWriter, r *http.Request) {
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: "invalid user id",
		})

		return
	}

	user, err := u.service.UserInfo(r.Context(), userID)
	if err != nil {
		respondServiceError(w, err)
		return
	}

	response.RespondOK(w, dto.UserInfoResponse{
		ID:        user.ID.String(),
		Name:      user.Name,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt,
	})
}

func (u *User) setCookie(w http.ResponseWriter, path, name, value string, expires time.Time) {
	cookie := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}

	if u.env == config.EnvProduction {
		cookie.Secure = true
	}

	http.SetCookie(w, cookie)
}
