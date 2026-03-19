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
			name:       "gmail pubsub route",
			method:     "POST",
			path:       "/gmail-pubsub",
			wantAction: "pubsub.forward",
			wantFlow:   "gmail_pubsub_fanout",
		},
		{
			name:       "start claw route",
			method:     "POST",
			path:       "/claws/start",
			wantAction: "claw.start",
			wantFlow:   "claw_lifecycle",
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
