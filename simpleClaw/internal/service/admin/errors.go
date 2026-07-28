package admin

import "errors"

var (
	// ErrUserIDRequired is returned when a per-user query is called without a user id.
	ErrUserIDRequired = errors.New("admin: user id is required")
	// ErrClawIDRequired is returned when a claw query is called without a claw id.
	ErrClawIDRequired = errors.New("admin: claw id is required")
)
