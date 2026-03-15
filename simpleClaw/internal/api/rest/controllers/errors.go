package controllers

import (
	"errors"
	"net/http"
	"shared/pkg/response"

	"simpleClaw/internal/infra/openrouter"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/claw"
	"simpleClaw/internal/service/user"
)

func respondServiceError(w http.ResponseWriter, err error) {
	code, message := mapServiceError(err)
	response.RespondError(w, response.Error{
		Code:    code,
		Message: message,
	})
}

type serviceErrorMapping struct {
	err     error
	code    int
	message string
}

var serviceErrorMappings = []serviceErrorMapping{
	{err: sql.ErrNotFound, code: http.StatusNotFound, message: "not found"},
	{err: claw.ErrChannelNotFound, code: http.StatusNotFound, message: "channel not found"},

	{err: sql.ErrConflict, code: http.StatusConflict, message: "resource already exists"},
	{err: sql.ErrInvalid, code: http.StatusBadRequest, message: "invalid data"},

	{err: user.ErrChannelUnsupported, code: http.StatusBadRequest, message: "channel type is not supported"},
	{err: claw.ErrUserIDRequired, code: http.StatusBadRequest, message: "user id is required"},
	{err: claw.ErrClawIDRequired, code: http.StatusBadRequest, message: "claw id is required"},
	{err: claw.ErrNameRequired, code: http.StatusBadRequest, message: "name is required"},
	{err: claw.ErrModelRequired, code: http.StatusBadRequest, message: "model is required"},
	{err: claw.ErrServerIDRequired, code: http.StatusBadRequest, message: "server id is required"},
	{err: claw.ErrContainerIDRequired, code: http.StatusBadRequest, message: "container id is required"},
	{err: claw.ErrPairingCodeRequired, code: http.StatusBadRequest, message: "code is required"},
	{err: openrouter.ErrModelRequired, code: http.StatusBadRequest, message: "model is required"},
	{err: openrouter.ErrModelNotFound, code: http.StatusBadRequest, message: "model not found"},
	{err: openrouter.ErrBadRequest, code: http.StatusBadRequest, message: "openrouter request is invalid"},

	{err: openrouter.ErrRateLimited, code: http.StatusTooManyRequests, message: "openrouter rate limit exceeded"},

	{err: sql.ErrUnavailable, code: http.StatusServiceUnavailable, message: "storage unavailable"},
	{err: claw.ErrHostingMissing, code: http.StatusServiceUnavailable, message: "hosting is not configured"},
	{err: claw.ErrOpenRouterClient, code: http.StatusServiceUnavailable, message: "openrouter is not configured"},
	{err: claw.ErrConfigArchivePathRequired, code: http.StatusServiceUnavailable, message: "config archive path is not configured"},
	{err: openrouter.ErrMissingBaseURL, code: http.StatusServiceUnavailable, message: "openrouter is not configured"},
	{err: openrouter.ErrMissingAPIToken, code: http.StatusServiceUnavailable, message: "openrouter is not configured"},
	{err: openrouter.ErrUnavailable, code: http.StatusServiceUnavailable, message: "openrouter is unavailable"},

	{err: openrouter.ErrUnauthorized, code: http.StatusBadGateway, message: "openrouter credentials are invalid"},
	{err: openrouter.ErrNotFound, code: http.StatusBadGateway, message: "openrouter resource not found"},
}

func mapServiceError(err error) (int, string) {
	if err == nil {
		return http.StatusInternalServerError, "internal error"
	}

	for _, mapping := range serviceErrorMappings {
		if errors.Is(err, mapping.err) {
			return mapping.code, mapping.message
		}
	}

	return http.StatusInternalServerError, "internal error"
}
