package hosting

import "errors"

var (
	ErrInvalidCode               = errors.New("invalid code")
	ErrApproveChannelUnsupported = errors.New("approve channel is not supported")
	ErrServerCapacityExceeded    = errors.New("server capacity exceeded")
	ErrServerMemoryUnavailable   = errors.New("server memory unavailable")
	ErrRuntimeNotFound           = errors.New("runtime not found")
)
