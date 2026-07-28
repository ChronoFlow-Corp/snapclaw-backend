package user

import "errors"

var (
	ErrChannelUnsupported  = errors.New("channel is not supported")
	ErrProviderUnsupported = errors.New("provider is not supported")
	ErrAccessTokenRequired = errors.New("access token is required")
)
