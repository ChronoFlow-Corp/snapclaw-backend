package controllers

import (
	"errors"
	"net/http"
	"shared/pkg/response"

	"simpleClaw/internal/infra/hosting"
	"simpleClaw/internal/infra/openrouter"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/billing"
	"simpleClaw/internal/service/claw"
	"simpleClaw/internal/service/integrations/googleoauth"
	"simpleClaw/internal/service/server"
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
	{
		err:     sql.ErrNotFound,
		code:    http.StatusNotFound,
		message: "not found",
	},
	{
		err:     billing.ErrSubscriptionNotFound,
		code:    http.StatusNotFound,
		message: "subscription not found",
	},
	{
		err:     claw.ErrChannelNotFound,
		code:    http.StatusNotFound,
		message: "channel not found",
	},
	{
		err:     claw.ErrLifecycleOperationInProgress,
		code:    http.StatusConflict,
		message: "lifecycle operation already in progress",
	},
	{
		err:     sql.ErrConflict,
		code:    http.StatusConflict,
		message: "resource already exists",
	},
	{
		err:     billing.ErrPlanInactive,
		code:    http.StatusConflict,
		message: "plan is inactive",
	},
	{
		err:     billing.ErrSubscriptionChangeWhileActive,
		code:    http.StatusConflict,
		message: "subscription change is not allowed while current plan is active",
	},
	{
		err:     billing.ErrInsufficientBalance,
		code:    http.StatusConflict,
		message: "insufficient balance",
	},
	{
		err:     billing.ErrSubscriptionNotPending,
		code:    http.StatusConflict,
		message: "subscription is not pending",
	},
	{
		err:     billing.ErrPaymentNotPaid,
		code:    http.StatusConflict,
		message: "payment is not paid",
	},
	{
		err:     sql.ErrInvalid,
		code:    http.StatusBadRequest,
		message: "invalid data",
	},
	{
		err:     googleoauth.ErrUnsupportedCapability,
		code:    http.StatusBadRequest,
		message: "unsupported capability",
	},
	{
		err:     googleoauth.ErrReturnToInvalid,
		code:    http.StatusBadRequest,
		message: "returnTo is invalid",
	},
	{
		err:     billing.ErrPaymentEventTypeInvalid,
		code:    http.StatusBadRequest,
		message: "invalid payment event type",
	},
	{
		err:     billing.ErrOpenRouterAPIKeyInvalid,
		code:    http.StatusBadRequest,
		message: "invalid openrouter api key",
	},
	{
		err:     billing.ErrUnsupportedCurrency,
		code:    http.StatusBadRequest,
		message: "unsupported currency",
	},

	{
		err:     user.ErrChannelUnsupported,
		code:    http.StatusBadRequest,
		message: "channel type is not supported",
	},
	{
		err:     user.ErrProviderUnsupported,
		code:    http.StatusBadRequest,
		message: "provider is not supported",
	},
	{
		err:     user.ErrAccessTokenRequired,
		code:    http.StatusBadRequest,
		message: "access token is required",
	},
	{
		err:     claw.ErrUserIDRequired,
		code:    http.StatusBadRequest,
		message: "user id is required",
	},
	{
		err:     claw.ErrClawIDRequired,
		code:    http.StatusBadRequest,
		message: "claw id is required",
	},
	{
		err:     claw.ErrNameRequired,
		code:    http.StatusBadRequest,
		message: "name is required",
	},
	{
		err:     claw.ErrModelRequired,
		code:    http.StatusBadRequest,
		message: "model is required",
	},
	{
		err:     claw.ErrWebSearchRequired,
		code:    http.StatusBadRequest,
		message: "web search capability is required",
	},
	{
		err:     claw.ErrWebSearchProviderUnsupported,
		code:    http.StatusBadRequest,
		message: "web search provider is not supported",
	},
	{
		err:     server.ErrServerIDRequired,
		code:    http.StatusBadRequest,
		message: "server id is required",
	},
	{
		err:     server.ErrNameRequired,
		code:    http.StatusBadRequest,
		message: "name is required",
	},
	{
		err:     server.ErrURLRequired,
		code:    http.StatusBadRequest,
		message: "server url is required",
	},
	{
		err:     server.ErrProxyURLInvalid,
		code:    http.StatusBadRequest,
		message: "server proxy url is invalid",
	},
	{
		err:     server.ErrSecretKeyRequired,
		code:    http.StatusBadRequest,
		message: "server secret key is required",
	},
	{
		err:     claw.ErrServerIDRequired,
		code:    http.StatusBadRequest,
		message: "server id is required",
	},
	{
		err:     claw.ErrContainerIDRequired,
		code:    http.StatusBadRequest,
		message: "container id is required",
	},
	{
		err:     claw.ErrPairingCodeRequired,
		code:    http.StatusBadRequest,
		message: "code is required",
	},
	{
		err:     claw.ErrPairingCodeInvalid,
		code:    http.StatusBadRequest,
		message: "invalid code",
	},
	{
		err:     claw.ErrApproveChannelRequired,
		code:    http.StatusBadRequest,
		message: "channel type is required",
	},
	{
		err:     claw.ErrApproveChannelUnsupported,
		code:    http.StatusBadRequest,
		message: "channel type is not supported",
	},
	{
		err:     claw.ErrProviderUnsupported,
		code:    http.StatusBadRequest,
		message: "provider is not supported",
	},
	{
		err:     claw.ErrGmailCapabilityRequired,
		code:    http.StatusBadRequest,
		message: "gmail capability is not attached",
	},
	{
		err:     claw.ErrGmailIntegrationRequired,
		code:    http.StatusBadRequest,
		message: "gmail integration is required",
	},
	{
		err:     openrouter.ErrModelRequired,
		code:    http.StatusBadRequest,
		message: "model is required",
	},
	{
		err:     openrouter.ErrModelNotFound,
		code:    http.StatusBadRequest,
		message: "model not found",
	},
	{
		err:     openrouter.ErrBadRequest,
		code:    http.StatusBadRequest,
		message: "openrouter request is invalid",
	},

	{
		err:     openrouter.ErrRateLimited,
		code:    http.StatusTooManyRequests,
		message: "openrouter rate limit exceeded",
	},
	{
		err:     hosting.ErrServerCapacityExceeded,
		code:    http.StatusConflict,
		message: "server capacity exceeded",
	},
	{
		err:     hosting.ErrServerMemoryUnavailable,
		code:    http.StatusConflict,
		message: "server memory unavailable",
	},

	{
		err:     sql.ErrUnavailable,
		code:    http.StatusServiceUnavailable,
		message: "storage unavailable",
	},
	{
		err:     claw.ErrHostingMissing,
		code:    http.StatusServiceUnavailable,
		message: "hosting is not configured",
	},
	{
		err:     claw.ErrOpenRouterClient,
		code:    http.StatusServiceUnavailable,
		message: "openrouter is not configured",
	},
	{
		err:     claw.ErrGmailWatchTopicRequired,
		code:    http.StatusServiceUnavailable,
		message: "gmail watch topic is not configured",
	},
	{
		err:     claw.ErrBraveAPIKeyMissing,
		code:    http.StatusServiceUnavailable,
		message: "brave api key is not configured",
	},
	{
		err:     openrouter.ErrMissingBaseURL,
		code:    http.StatusServiceUnavailable,
		message: "openrouter is not configured",
	},
	{
		err:     openrouter.ErrMissingAPIToken,
		code:    http.StatusServiceUnavailable,
		message: "openrouter is not configured",
	},
	{
		err:     openrouter.ErrUnavailable,
		code:    http.StatusServiceUnavailable,
		message: "openrouter is unavailable",
	},

	{
		err:     openrouter.ErrUnauthorized,
		code:    http.StatusBadGateway,
		message: "openrouter credentials are invalid",
	},
	{
		err:     openrouter.ErrNotFound,
		code:    http.StatusBadGateway,
		message: "openrouter resource not found",
	},
	{
		err:     hosting.ErrServerCapacityExceeded,
		code:    http.StatusUnprocessableEntity,
		message: "server capacity exceeded",
	},
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
