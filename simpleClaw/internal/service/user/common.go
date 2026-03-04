package user

import (
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
	case channels.DmAllowList:
		tgCh.DmPolicy = channels.DmAllowList
		tgCh.AllowFrom = cm.AllowFrom
	default:
		tgCh.DmPolicy = channels.DmPairing
	}

	tgCh.BotToken = cm.BotToken

	return tgCh
}
