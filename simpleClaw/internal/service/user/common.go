package user

import (
	"strings"

	"simpleClaw/internal/entities/channels"
	"simpleClaw/internal/service/user/commands"
)

func addTgCfg(cm commands.TelegramChannel) channels.TelegramConfig {
	tgCh := channels.TelegramConfig{}

	switch channels.DmPolicy(cm.DmPolicy) {
	case channels.DmDisabled:
		tgCh.DmPolicy = channels.DmDisabled
	case channels.DmOpen:
		tgCh.DmPolicy = channels.DmOpen

		allowFrom := normalizeAllowFrom(cm.AllowFrom)
		if !containsAllowAll(allowFrom) {
			allowFrom = append(allowFrom, "*")
		}

		tgCh.AllowFrom = allowFrom
		tgCh.GroupPolicy = "allowlist"
		tgCh.Groups = map[string]interface{}{"*": struct {
			RequireMention bool `json:"requireMention"`
		}{
			RequireMention: true,
		}}
	case channels.DmAllowList:
		tgCh.DmPolicy = channels.DmAllowList
		tgCh.AllowFrom = normalizeAllowFrom(cm.AllowFrom)
	default:
		tgCh.DmPolicy = channels.DmPairing
	}

	tgCh.BotToken = cm.BotToken

	return tgCh
}

func normalizeAllowFrom(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, raw := range values {
		val := strings.TrimSpace(raw)
		if val == "" {
			continue
		}

		if _, ok := seen[val]; ok {
			continue
		}

		seen[val] = struct{}{}
		out = append(out, val)
	}

	return out
}

func containsAllowAll(values []string) bool {
	for _, val := range values {
		if val == "*" {
			return true
		}
	}

	return false
}
