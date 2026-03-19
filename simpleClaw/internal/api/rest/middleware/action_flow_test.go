package middleware

import "testing"

func TestClassifySimpleClawActionFlow(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		wantAction string
		wantFlow   string
	}{
		{
			name:       "create claw",
			method:     "POST",
			path:       "/api/claws",
			wantAction: "claw.create",
			wantFlow:   "claw_lifecycle",
		},
		{
			name:       "start claw by path",
			method:     "POST",
			path:       "/api/claws/123/start",
			wantAction: "claw.start",
			wantFlow:   "claw_lifecycle",
		},
		{
			name:       "list servers",
			method:     "GET",
			path:       "/api/servers",
			wantAction: "server.list",
			wantFlow:   "server_registry",
		},
		{
			name:       "pubsub proxy",
			method:     "POST",
			path:       "/pubsub",
			wantAction: "pubsub.forward",
			wantFlow:   "gmail_pubsub_fanout",
		},
		{
			name:       "health",
			method:     "GET",
			path:       "/health",
			wantAction: "health.check",
			wantFlow:   "health",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, flow := classifySimpleClawActionFlow(tt.method, tt.path)
			if action != tt.wantAction {
				t.Fatalf("action = %q, want %q", action, tt.wantAction)
			}
			if flow != tt.wantFlow {
				t.Fatalf("flow = %q, want %q", flow, tt.wantFlow)
			}
		})
	}
}
