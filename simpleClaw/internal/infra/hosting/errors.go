package hosting

import "errors"

var (
	ErrInvalidCode             = errors.New("invalid code")
	ErrServerCapacityExceeded  = errors.New("server capacity exceeded")
	ErrServerMemoryUnavailable = errors.New("server memory unavailable")
)
