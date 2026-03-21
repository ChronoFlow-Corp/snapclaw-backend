package middleware

import "testing"

func TestClassifyActionFlow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantAction string
		wantFlow   string
	}{
		{
			name:       "claw start route",
			method:     "POST",
			path:       "/api/claws/123/start",
			wantAction: "claw.start",
			wantFlow:   "claw_lifecycle",
		},
		{
			name:       "gmail callback route",
			method:     "GET",
			path:       "/api/me/connect/gmail/callback?code=1",
			wantAction: "channel.connect.callback",
			wantFlow:   "channel_connect",
		},
		{
			name:       "unknown route",
			method:     "GET",
			path:       "/unknown",
			wantAction: "http.request",
			wantFlow:   "http_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotAction, gotFlow := ClassifyActionFlow(tt.method, tt.path)
			if gotAction != tt.wantAction {
				t.Fatalf("action = %q, want %q", gotAction, tt.wantAction)
			}

			if gotFlow != tt.wantFlow {
				t.Fatalf("flow = %q, want %q", gotFlow, tt.wantFlow)
			}
		})
	}
}
