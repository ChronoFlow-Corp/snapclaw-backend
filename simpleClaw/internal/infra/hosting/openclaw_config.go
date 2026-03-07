package hosting

import (
	"reflect"

	"simpleClaw/internal/entities"
)

type openClawConfig struct {
	Env      *entities.Env           `json:"env,omitempty"`
	Channels *entities.ClawChannels  `json:"channels,omitempty"`
	Agents   *openClawAgents         `json:"agents,omitempty"`
	Gateway  *entities.GatewayConfig `json:"gateway,omitempty"`
	Meta     *entities.ConfigMeta    `json:"meta,omitempty"`
}

type openClawAgents struct {
	Defaults *entities.AgentDefaults `json:"defaults,omitempty"`
	List     []openClawAgent         `json:"list,omitempty"`
}

type openClawAgent struct {
	ID       string             `json:"id"`
	Default  bool               `json:"default,omitempty"`
	Identity *entities.Identity `json:"identity,omitempty"`
}

func buildOpenClawConfig(cfg entities.ClawConfig) openClawConfig {
	var out openClawConfig

	if !isZero(cfg.Env) {
		env := cfg.Env
		out.Env = &env
	}

	if !isZero(cfg.Channels) {
		channels := cfg.Channels
		out.Channels = &channels
	}

	if agents := buildOpenClawAgents(cfg); agents != nil {
		out.Agents = agents
	}

	if !isZero(cfg.Gateway) {
		gateway := cfg.Gateway
		out.Gateway = &gateway
	}

	if !isZero(cfg.Meta) {
		meta := cfg.Meta
		out.Meta = &meta
	}

	return out
}

func buildOpenClawAgents(cfg entities.ClawConfig) *openClawAgents {
	var out openClawAgents

	if !isZero(cfg.Agents.Defaults) {
		defaults := cfg.Agents.Defaults
		out.Defaults = &defaults
	}

	if !isZero(cfg.Identity) {
		identity := cfg.Identity
		out.List = append(out.List, openClawAgent{
			ID:       "main",
			Default:  true,
			Identity: &identity,
		})
	}

	if out.Defaults == nil && len(out.List) == 0 {
		return nil
	}

	return &out
}

func isZero[T any](value T) bool {
	return reflect.ValueOf(value).IsZero()
}
