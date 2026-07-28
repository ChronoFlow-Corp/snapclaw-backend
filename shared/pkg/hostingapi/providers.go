package hostingapi

import (
	"errors"
	"fmt"
	"shared/consts"
	"strings"
)

const (
	ProviderGmail          = consts.ProviderGmail
	ApproveChannelTelegram = "telegram"
	ApproveChannelWhatsApp = "whatsapp"
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

func NormalizeApproveChannelType(raw string) (string, error) {
	channelType := strings.ToLower(strings.TrimSpace(raw))

	switch channelType {
	case ApproveChannelTelegram, ApproveChannelWhatsApp:
		return channelType, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrProviderUnsupported, strings.TrimSpace(raw))
	}
}
