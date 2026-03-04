package jwt

import "errors"

var (
	ErrExpired          = errors.New("expired")
	ErrInvalid          = errors.New("invalid")
	ErrInvalidParseFunc = errors.New("parse func in args invalid")
)
