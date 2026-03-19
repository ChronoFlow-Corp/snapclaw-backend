package middleware

import "testing"

func TestClassifyContainerManagerActionFlow(t *testing.T) {
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
			path:       "/claws",
			wantAction: "claw.create",
			wantFlow:   "claw_lifecycle",
		},
		{
			name:       "approve",
			method:     "GET",
			path:       "/approve",
			wantAction: "claw.approve",
			wantFlow:   "claw_pairing",
		},
		{
			name:       "gmail pubsub",
			method:     "POST",
			path:       "/gmail-pubsub",
			wantAction: "pubsub.forward",
			wantFlow:   "gmail_pubsub_fanout",
		},
		{
			name:       "connect",
			method:     "POST",
			path:       "/connect",
			wantAction: "claw.connect",
			wantFlow:   "claw_pairing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, flow := classifyContainerManagerActionFlow(tt.method, tt.path)
			if action != tt.wantAction {
				t.Fatalf("action = %q, want %q", action, tt.wantAction)
			}
			if flow != tt.wantFlow {
				t.Fatalf("flow = %q, want %q", flow, tt.wantFlow)
			}
		})
	}
}
