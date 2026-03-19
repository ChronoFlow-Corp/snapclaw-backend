package hostingapi

import (
	"errors"
	"fmt"
	"shared/consts"
	"strings"
)

const (
	ProviderGmail = consts.ProviderGmail
)

var ErrProviderUnsupported = errors.New("provider is not supported")

func NormalizeProvider(raw string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(raw))

	switch provider {
	case ProviderGmail:
		return provider, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrProviderUnsupported, strings.TrimSpace(raw))
	}
}
