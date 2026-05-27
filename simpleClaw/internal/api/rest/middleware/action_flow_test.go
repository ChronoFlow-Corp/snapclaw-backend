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
			name:       "integration connect route",
			method:     "POST",
			path:       "/api/me/integrations/gmail/connect",
			wantAction: "integration.connect",
			wantFlow:   "account_integration",
		},
		{
			name:       "yookassa webhook route",
			method:     "POST",
			path:       "/api/billing/webhook/yookassa",
			wantAction: "payment.webhook.receive",
			wantFlow:   "billing_payment",
		},
		{
			name:       "openrouter webhook route",
			method:     "POST",
			path:       "/api/billing/webhook/openrouter",
			wantAction: "usage.webhook.receive",
			wantFlow:   "billing_usage",
		},
		{
			name:       "telegram manager link route",
			method:     "POST",
			path:       "/api/me/telegram/manager/link",
			wantAction: "telegram.manager.link.create",
			wantFlow:   "telegram_manager",
		},
		{
			name:       "telegram manager webhook route",
			method:     "POST",
			path:       "/api/telegram/manager/webhook",
			wantAction: "telegram.manager.webhook.receive",
			wantFlow:   "telegram_manager",
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
