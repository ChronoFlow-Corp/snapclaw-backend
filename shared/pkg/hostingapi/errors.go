package hostingapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	ErrCodeValidation  = "validation"
	ErrCodeInvalidCode = "invalid_code"
	ErrCodeNotFound    = "not_found"
	ErrCodeInternal    = "internal"
)

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	if status < http.StatusBadRequest {
		status = http.StatusInternalServerError
	}

	message = strings.TrimSpace(message)
	if message == "" {
		message = http.StatusText(status)
	}

	code = strings.TrimSpace(code)
	if code == "" {
		code = ErrCodeInternal
	}

	resp := ErrorResponse{
		Code:    code,
		Message: message,
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func ParseErrorResponse(raw []byte) (ErrorResponse, bool) {
	var parsed ErrorResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ErrorResponse{}, false
	}

	parsed.Code = strings.TrimSpace(parsed.Code)
	parsed.Message = strings.TrimSpace(parsed.Message)
	if parsed.Code == "" || parsed.Message == "" {
		return ErrorResponse{}, false
	}

	return parsed, true
}
