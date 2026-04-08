package hosting

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"shared/pkg/hostingapi"
	"simpleClaw/internal/entities"
)

func TestClientEnsureRuntime_UsesEnsureEndpointAndDecodesResponse(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotReq hostingapi.EnsureRuntimeRequest

	c := &client{
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotPath = req.URL.Path
				if err := json.NewDecoder(req.Body).Decode(&gotReq); err != nil {
					t.Fatalf("decode request: %v", err)
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body: io.NopCloser(strings.NewReader(
						`{"runtimeRecordId":"runtime-1","dockerContainerId":"docker-1","port":"8080"}`,
					)),
					Header: make(http.Header),
				}, nil
			}),
		},
	}

	resp, err := c.ensureRuntime(context.Background(), "http://container-manager.local", nil, hostingapi.EnsureRuntimeRequest{
		UserID: "user-1",
		ClawID: "claw-1",
		ClawConfig: []hostingapi.ClawConfigFile{
			{Name: "openclaw", FileType: "json", Data: "{}"},
		},
	})
	if err != nil {
		t.Fatalf("ensureRuntime() error = %v", err)
	}

	if gotPath != hostingapi.ClawsEnsureEndpoint {
		t.Fatalf("path = %q, want %q", gotPath, hostingapi.ClawsEnsureEndpoint)
	}

	if gotReq.ClawID != "claw-1" || gotReq.UserID != "user-1" {
		t.Fatalf("request = %+v", gotReq)
	}

	if resp.RuntimeRecordID != "runtime-1" {
		t.Fatalf("runtimeRecordId = %q, want %q", resp.RuntimeRecordID, "runtime-1")
	}
}

func TestBuildConfigFiles_IncludesRuntimeBindingsWhenConfigured(t *testing.T) {
	t.Parallel()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Hooks = &entities.HooksConfig{
		Gmail: entities.GmailHookConfig{
			Serve: entities.ServeConfig{
				Path: "/gmail-pubsub",
			},
		},
	}

	files, err := buildConfigFiles(cfg)
	if err != nil {
		t.Fatalf("buildConfigFiles() error = %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2", len(files))
	}

	if files[1].Name != runtimeBindingsName {
		t.Fatalf("runtime binding file name = %q, want %q", files[1].Name, runtimeBindingsName)
	}

	if !strings.Contains(files[1].Data, `"gmail-pubsub"`) {
		t.Fatalf("runtime binding data = %s", files[1].Data)
	}
}

func TestBuildVars_UsesRuntimeBraveSecretInsteadOfPlaceholder(t *testing.T) {
	t.Parallel()

	cfg := entities.NewDefaultClawConfig("openrouter/openai/gpt-4.1-mini")
	cfg.Env.Vars = map[string]string{
		"BRAVE_API_KEY": "${BRAVE_API_KEY}",
	}

	vars := buildVars(cfg, RuntimeSecrets{BraveAPIKey: "brave-secret-1"})
	if len(vars) != 1 {
		t.Fatalf("vars len = %d, want 1", len(vars))
	}

	if vars[0] != "BRAVE_API_KEY=brave-secret-1" {
		t.Fatalf("vars[0] = %q, want %q", vars[0], "BRAVE_API_KEY=brave-secret-1")
	}
}
