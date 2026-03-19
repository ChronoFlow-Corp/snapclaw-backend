package controllers

import (
	"errors"
	"net/http"

	"containermanager/internal/service"
)

func mapApproveError(err error) (int, string) {
	if errors.Is(err, service.ErrInvalidCode) {
		return http.StatusBadRequest, service.ErrInvalidCode.Error()
	}

	return http.StatusInternalServerError, err.Error()
}
