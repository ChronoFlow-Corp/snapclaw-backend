package controllers

import (
	"errors"
	"net/http"
	"shared/pkg/hostingapi"

	"containermanager/internal/service"
)

func mapApproveError(err error) (int, hostingapi.ErrorResponse) {
	if errors.Is(err, service.ErrInvalidCode) {
		return http.StatusBadRequest, hostingapi.ErrorResponse{
			Code:    hostingapi.ErrCodeInvalidCode,
			Message: service.ErrInvalidCode.Error(),
		}
	}

	return http.StatusInternalServerError, hostingapi.ErrorResponse{
		Code:    hostingapi.ErrCodeInternal,
		Message: err.Error(),
	}
}
