package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"shared/pkg/jwt"
	"shared/pkg/response"
	"time"

	"simpleClaw/internal/api/rest/dto"
	"simpleClaw/internal/entities"

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

	AddChannel(
		ctx context.Context,
		cm commands.AddChannel,
	) (entities.Channel, error)
}
type User struct {
	env     string
	service service
	j       jwt.JWT
}

func NewUser(env string, service service) *User {
	return &User{
		env:     env,
		service: service,
	}
}

func (u *User) Register(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Get("/connect/{provider}", u.Login)
		r.Get("/connect/{provider}/callback", u.Callback)
	})

	r.Group(func(r chi.Router) {
		// r.Use(middleware.AuthJwt(u.j))
		r.Post("/channel", u.AddChannel)
	})
}

func (u *User) Login(w http.ResponseWriter, r *http.Request) {
	if gothUser, err := gothic.CompleteUserAuth(w, r); err == nil {
		access, refresh, err := u.service.SignIn(r.Context(), commands.SignIn{
			Name:  gothUser.NickName,
			Email: gothUser.Email,
		})
		if err != nil {
			response.RespondError(w, response.Error{
				Code:    http.StatusUnauthorized,
				Message: err.Error(),
			})

			return
		}

		u.setCookie(w, "/", "access_token", access.Raw, access.Claims.ExpiresAt.Time)
		u.setCookie(w, "/", "refresh_token", refresh.Raw, refresh.Claims.ExpiresAt.Time)

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
		Name:  gothUser.NickName,
		Email: gothUser.Email,
	})
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusUnauthorized,
			Message: err.Error(),
		})

		return
	}

	u.setCookie(w, "/", "access_token", access.Raw, access.Claims.ExpiresAt.Time)
	u.setCookie(w, "/", "refresh_token", refresh.Raw, refresh.Claims.ExpiresAt.Time)
}

func (u *User) AddChannel(w http.ResponseWriter, r *http.Request) {
	var req dto.AddChannelRequest

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		response.RespondError(w, response.Error{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})

		return
	}

	//userID, ok := r.Context().Value("user_id").(uuid.UUID)
	//if !ok {
	//	response.RespondError(w, response.Error{
	//		Code:    http.StatusBadRequest,
	//		Message: "User ID should be a UUID",
	//	})
	//
	//	return
	//}

	userID, _ := uuid.Parse("d3612cab-4fbd-465d-85a6-6c360afc8812")

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
		response.RespondError(w, response.Error{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
	}

	response.RespondOK(w, ch)
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
