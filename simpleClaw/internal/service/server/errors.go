package server

import "errors"

var (
	ErrServerIDRequired  = errors.New("server id is required")
	ErrNameRequired      = errors.New("name is required")
	ErrURLRequired       = errors.New("url is required")
	ErrProxyURLInvalid   = errors.New("proxy url is invalid")
	ErrSecretKeyRequired = errors.New("secret key is required")
)
