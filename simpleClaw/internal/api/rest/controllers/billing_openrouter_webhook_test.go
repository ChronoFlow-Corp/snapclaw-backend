package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBillingHandleOpenRouterWebhook(t *testing.T) {
	t.Parallel()

	t.Run("forwards valid usage payload to billing service", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/openrouter",
			strings.NewReader(`{
				"resourceSpans": [{
					"scopeSpans": [{
						"spans": [{
							"traceId": "trace-1",
							"spanId": "span-1",
							"endTimeUnixNano": "1774526400000000000",
							"attributes": [
								{"key":"trace.metadata.openrouter.api_key_name","value":{"stringValue":"snapclaw+2dd0b4f0-6f6a-4f4c-b4de-8e5fa6db6d66"}},
								{"key":"gen_ai.response.model","value":{"stringValue":"openai/gpt-4.1-mini"}},
								{"key":"gen_ai.usage.total_cost","value":{"doubleValue":0.12}}
							]
						}]
					}]
				}]
			}`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", env.openRouterWebhookSecret)

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}

		if env.billingService.lastOpenRouterWebhook.TraceID != "trace-1" {
			t.Fatalf("trace id = %q, want %q", env.billingService.lastOpenRouterWebhook.TraceID, "trace-1")
		}

		if env.billingService.lastOpenRouterWebhook.SpanID != "span-1" {
			t.Fatalf("span id = %q, want %q", env.billingService.lastOpenRouterWebhook.SpanID, "span-1")
		}

		if env.billingService.lastOpenRouterWebhook.APIKeyName != "snapclaw+2dd0b4f0-6f6a-4f4c-b4de-8e5fa6db6d66" {
			t.Fatalf("api key name = %q", env.billingService.lastOpenRouterWebhook.APIKeyName)
		}

		if env.billingService.lastOpenRouterWebhook.TotalCost != "0.12" {
			t.Fatalf("total cost = %q, want %q", env.billingService.lastOpenRouterWebhook.TotalCost, "0.12")
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/openrouter",
			strings.NewReader(`{"resourceSpans":`),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", env.openRouterWebhookSecret)

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns 401 for missing auth header", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/openrouter",
			strings.NewReader(`{"resourceSpans":[]}`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("returns 415 for non json content type", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/openrouter",
			strings.NewReader(`resourceSpans=`),
		)
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Authorization", env.openRouterWebhookSecret)

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnsupportedMediaType)
		}
	})
}
