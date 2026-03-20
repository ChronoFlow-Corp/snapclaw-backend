package entities

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNewCreateClawConfig_UsesAgentsListOnly(t *testing.T) {
	cfg := NewCreateClawConfig(CreateClawConfigInput{
		PrimaryModel: "openrouter/openai/gpt-4.1-mini",
	})

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	text := string(data)
	if strings.Contains(text, `"toolsRuntime"`) {
		t.Fatalf("expected unified tools section, got %s", text)
	}

	if strings.Contains(text, `"defaults"`) {
		t.Fatalf("expected agents.list only, got %s", text)
	}

	if !strings.Contains(text, `"tools"`) {
		t.Fatalf("expected tools section, got %s", text)
	}

	if !strings.Contains(text, `"list"`) {
		t.Fatalf("expected agents.list section, got %s", text)
	}
}

func TestNewDefaultMainAgentConfig_PreservesSubagentsAndTools(t *testing.T) {
	agent := NewDefaultMainAgentConfig("openrouter/openai/gpt-4.1-mini")

	if !agent.Default {
		t.Fatalf("expected default agent")
	}

	if agent.ID != "main" {
		t.Fatalf("expected agent ID main, got %q", agent.ID)
	}

	if agent.Subagents == nil {
		t.Fatalf("expected subagents config")
	}

	if len(agent.Subagents.AllowAgents) != 1 || agent.Subagents.AllowAgents[0] != "*" {
		t.Fatalf("expected wildcard subagent support, got %#v", agent.Subagents.AllowAgents)
	}

	if agent.Tools == nil {
		t.Fatalf("expected per-agent tools config")
	}

	wantAllow := []string{
		"group:fs",
		"group:sessions",
		"group:web",
		"group:messaging",
		"group:automation",
		"exec",
		"process",
	}

	if len(agent.Tools.Allow) != len(wantAllow) {
		t.Fatalf("expected %d allowed tools, got %#v", len(wantAllow), agent.Tools.Allow)
	}

	for i := range wantAllow {
		if agent.Tools.Allow[i] != wantAllow[i] {
			t.Fatalf("expected allowed tools %#v, got %#v", wantAllow, agent.Tools.Allow)
		}
	}

	wantDeny := []string{"canvas", "browser"}
	if len(agent.Tools.Deny) != len(wantDeny) {
		t.Fatalf("expected deny list %#v, got %#v", wantDeny, agent.Tools.Deny)
	}

	for i := range wantDeny {
		if agent.Tools.Deny[i] != wantDeny[i] {
			t.Fatalf("expected deny list %#v, got %#v", wantDeny, agent.Tools.Deny)
		}
	}

	if agent.Model == nil {
		t.Fatalf("expected model config")
	}

	if agent.Model.Primary != "openrouter/openai/gpt-4.1-mini" {
		t.Fatalf("expected primary model to be preserved, got %q", agent.Model.Primary)
	}
}

func TestNewCreateClawConfig_JSONContract(t *testing.T) {
	cfg := NewCreateClawConfig(CreateClawConfigInput{
		PrimaryModel: "openrouter/openai/gpt-4.1-mini",
	})

	gotJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	const wantJSON = `{
		"env": {
			"OPENROUTER_API_KEY": "${OPENROUTER_API_KEY}",
			"shellEnv": {
				"enabled": true,
				"timeoutMs": 30000
			}
		},
		"logging": {
			"level": "info",
			"consoleLevel": "info",
			"consoleStyle": "pretty"
		},
		"tools": {
			"media": {
				"audio": {
					"enabled": false,
					"maxBytes": 10485760,
					"timeoutSeconds": 60
				},
				"video": {
					"enabled": false,
					"maxBytes": 52428800
				}
			}
		},
		"session": {
			"scope": "per-sender",
			"reset": {
				"mode": "idle",
				"idleMinutes": 60
			},
			"store": "file",
			"maintenance": {
				"mode": "warn",
				"pruneAfter": "30d",
				"maxEntries": 1000
			},
			"typingIntervalSeconds": 3,
			"sendPolicy": {
				"default": "allow"
			}
		},
		"agents": {
			"list": [{
				"id": "main",
				"default": true,
				"name": "main",
				"workspace": "workspace/main",
				"agentDir": "agent/agents",
				"model": {
					"primary": "openrouter/openai/gpt-4.1-mini"
				},
				"groupChat": {
					"mentionPatterns": ["@OpenClaw"]
				},
				"sandbox": {
					"mode": "off"
				},
				"subagents": {
					"allowAgents": ["*"]
				},
				"tools": {
					"allow": [
						"group:fs",
						"group:sessions",
						"group:web",
						"group:messaging",
						"group:automation",
						"exec",
						"process"
					],
					"deny": ["canvas", "browser"]
				}
			}]
		},
		"cron": {
			"enabled": true,
			"maxConcurrentRuns": 2,
			"sessionRetention": "24h",
			"runLog": {
				"maxBytes": "2mb",
				"keepLines": 1000
			}
		},
		"gateway": {
			"mode": "local",
			"bind": "lan",
			"controlUi": {
				"enabled": false
			},
			"auth": {
				"mode": "token",
				"token": "${OPENCLAW_GATEWAY_TOKEN}"
			},
			"reload": {
				"mode": "restart",
				"debounceMs": 2000
			}
		}
	}`

	var got any
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("unmarshal got json: %v", err)
	}

	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("unmarshal want json: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected json contract\nwant: %s\ngot:  %s", wantJSON, string(gotJSON))
	}
}
