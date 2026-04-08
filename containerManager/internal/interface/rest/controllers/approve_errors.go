package controllers

import (
	"errors"
	"net/http"

	"containermanager/internal/service"
	"shared/pkg/hostingapi"
)

func mapApproveError(err error) (int, hostingapi.ErrorResponse) {
	if errors.Is(err, service.ErrInvalidCode) {
		return http.StatusBadRequest, hostingapi.ErrorResponse{
			Code:    hostingapi.ErrCodeInvalidCode,
			Message: service.ErrInvalidCode.Error(),
		}
	}

	if errors.Is(err, service.ErrApproveChannelUnsupported) {
		return http.StatusBadRequest, hostingapi.ErrorResponse{
			Code:    hostingapi.ErrCodeValidation,
			Message: service.ErrApproveChannelUnsupported.Error(),
		}
	}

	return http.StatusInternalServerError, hostingapi.ErrorResponse{
		Code:    hostingapi.ErrCodeInternal,
		Message: err.Error(),
	}
}
