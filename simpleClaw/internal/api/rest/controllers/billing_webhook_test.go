package controllers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shared/pkg/response"
	billingservice "simpleClaw/internal/service/billing"
)

func TestBillingHandleYooKassaWebhook(t *testing.T) {
	t.Parallel()

	t.Run("forwards valid notification to billing service", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/yookassa",
			strings.NewReader(`{
				"type":"notification",
				"event":"payment.succeeded",
				"object":{
					"id":"pay_123",
					"status":"succeeded",
					"paid":true,
					"amount":{"value":"1000.00","currency":"RUB"},
					"created_at":"2026-03-25T10:00:00Z"
				}
			}`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}

		if env.billingService.lastWebhook.Type != "notification" {
			t.Fatalf("type = %q, want %q", env.billingService.lastWebhook.Type, "notification")
		}

		if env.billingService.lastWebhook.Event != "payment.succeeded" {
			t.Fatalf("event = %q, want %q", env.billingService.lastWebhook.Event, "payment.succeeded")
		}

		if env.billingService.lastWebhook.Object.ID != "pay_123" {
			t.Fatalf("payment id = %q, want %q", env.billingService.lastWebhook.Object.ID, "pay_123")
		}
	})

	t.Run("returns 400 for invalid json", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/yookassa",
			strings.NewReader(`{"type":"notification"`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns 400 for invalid payment payload", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/yookassa",
			strings.NewReader(`{"type":"notification","event":"payment.succeeded","object":"bad"}`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("returns 415 for non json content type", func(t *testing.T) {
		env := newTestEnv(t)

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/yookassa",
			strings.NewReader(`type=notification`),
		)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnsupportedMediaType)
		}
	})

	t.Run("maps invalid payment event type error", func(t *testing.T) {
		env := newTestEnv(t)
		env.billingService.webhookErr = billingservice.ErrPaymentEventTypeInvalid

		req := httptest.NewRequest(
			http.MethodPost,
			"/api/billing/webhook/yookassa",
			strings.NewReader(`{
				"type":"notification",
				"event":"payment.succeeded",
				"object":{
					"id":"pay_123",
					"status":"succeeded",
					"paid":true,
					"amount":{"value":"1000.00","currency":"RUB"},
					"created_at":"2026-03-25T10:00:00Z"
				}
			}`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		env.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}

		var payload response.Error
		decodeJSON(t, rr, &payload)

		if payload.Message != "invalid payment event type" {
			t.Fatalf("message = %q, want %q", payload.Message, "invalid payment event type")
		}
	})
}
