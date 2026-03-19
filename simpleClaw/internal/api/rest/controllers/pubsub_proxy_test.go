package controllers_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"simpleClaw/internal/api/rest/controllers"

	"github.com/go-chi/chi/v5"
)

type proxiedRequest struct {
	Body                 string
	ContentType          string
	UserAgent            string
	XGoogTopic           string
	XGoogDeliveryAttempt string
	Authorization        string
	Token                string
}

func TestPubSubProxyHealth(t *testing.T) {
	router := chi.NewRouter()
	controllers.NewPubSubProxy(newFakeServerService(), controllers.PubSubProxyOptions{}).Register(router)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
	if strings.TrimSpace(rr.Body.String()) != "OK" {
		t.Fatalf("unexpected body: %q", rr.Body.String())
	}
}

func TestPubSubProxyFanOut(t *testing.T) {
	t.Run("returns 200 when at least one downstream succeeds", func(t *testing.T) {
		firstCh := make(chan proxiedRequest, 1)
		secondCh := make(chan proxiedRequest, 1)

		firstTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read first target body: %v", err)
			}

			firstCh <- proxiedRequest{
				Body:                 string(body),
				ContentType:          r.Header.Get("Content-Type"),
				UserAgent:            r.Header.Get("User-Agent"),
				XGoogTopic:           r.Header.Get("X-Goog-Topic"),
				XGoogDeliveryAttempt: r.Header.Get("X-Goog-Delivery-Attempt"),
				Authorization:        r.Header.Get("Authorization"),
				Token:                r.URL.Query().Get("token"),
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer firstTarget.Close()

		secondTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read second target body: %v", err)
			}

			secondCh <- proxiedRequest{
				Body:                 string(body),
				ContentType:          r.Header.Get("Content-Type"),
				UserAgent:            r.Header.Get("User-Agent"),
				XGoogTopic:           r.Header.Get("X-Goog-Topic"),
				XGoogDeliveryAttempt: r.Header.Get("X-Goog-Delivery-Attempt"),
				Authorization:        r.Header.Get("Authorization"),
				Token:                r.URL.Query().Get("token"),
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer secondTarget.Close()

		service := newFakeServerService()
		first := service.seed("first")
		first.ProxyURL = firstTarget.URL + "/gmail-pubsub"
		first.SecretKey = "downstream-secret-1"
		service.servers[first.ID] = first

		second := service.seed("second")
		second.ProxyURL = secondTarget.URL + "/gmail-pubsub"
		second.SecretKey = "downstream-secret-2"
		service.servers[second.ID] = second

		router := chi.NewRouter()
		controllers.NewPubSubProxy(service, controllers.PubSubProxyOptions{
			Token:          "ingress-secret",
			ForwardTimeout: time.Second,
		}).Register(router)

		rawBody := "{\n  \"message\": {\"data\": \"abc\"}\n}\n"

		req := httptest.NewRequest(
			http.MethodPost,
			"/pubsub?token=ingress-secret",
			strings.NewReader(rawBody),
		)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "pubsub-client/1.0")
		req.Header.Set("X-Goog-Topic", "projects/demo/topics/test")
		req.Header.Set("X-Goog-Delivery-Attempt", "3")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}

		firstReq := waitForProxyRequest(t, firstCh)
		secondReq := waitForProxyRequest(t, secondCh)

		assertForwardedRequest(t, firstReq, rawBody, "downstream-secret-1")
		assertForwardedRequest(t, secondReq, rawBody, "downstream-secret-2")
	})

	t.Run("returns 502 when all downstreams fail", func(t *testing.T) {
		targetA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer targetA.Close()

		targetB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer targetB.Close()

		service := newFakeServerService()
		first := service.seed("first")
		first.ProxyURL = targetA.URL + "/gmail-pubsub"
		service.servers[first.ID] = first

		second := service.seed("second")
		second.ProxyURL = targetB.URL + "/gmail-pubsub"
		service.servers[second.ID] = second

		router := chi.NewRouter()
		controllers.NewPubSubProxy(service, controllers.PubSubProxyOptions{
			ForwardTimeout: time.Second,
		}).Register(router)

		req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(`{"message":{}}`))
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("rejects invalid ingress token", func(t *testing.T) {
		var called atomic.Bool

		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called.Store(true)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()

		service := newFakeServerService()
		srv := service.seed("first")
		srv.ProxyURL = target.URL + "/gmail-pubsub"
		service.servers[srv.ID] = srv

		router := chi.NewRouter()
		controllers.NewPubSubProxy(service, controllers.PubSubProxyOptions{
			Token:          "expected-ingress-token",
			ForwardTimeout: time.Second,
		}).Register(router)

		req := httptest.NewRequest(
			http.MethodPost,
			"/pubsub?token=wrong-token",
			strings.NewReader(`{"message":{}}`),
		)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
		}
		if called.Load() {
			t.Fatalf("downstream target must not be called on invalid ingress token")
		}
	})
}

func TestPubSubProxyAcceptsIngressTokenFromAuthorizationHeader(t *testing.T) {
	service := newFakeServerService()
	router := chi.NewRouter()
	controllers.NewPubSubProxy(service, controllers.PubSubProxyOptions{
		Token:          "expected-ingress-token",
		ForwardTimeout: time.Second,
	}).Register(router)

	req := httptest.NewRequest(http.MethodPost, "/pubsub", strings.NewReader(`{"message":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer expected-ingress-token")

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}
}

func waitForProxyRequest(t *testing.T, ch <-chan proxiedRequest) proxiedRequest {
	t.Helper()

	select {
	case req := <-ch:
		return req
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for proxied request")
		return proxiedRequest{}
	}
}

func assertForwardedRequest(t *testing.T, req proxiedRequest, expectedBody, expectedAuthorization string) {
	t.Helper()

	if req.Body != expectedBody {
		t.Fatalf("unexpected body: %q", req.Body)
	}
	if req.ContentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", req.ContentType)
	}
	if req.UserAgent != "pubsub-client/1.0" {
		t.Fatalf("unexpected user agent: %q", req.UserAgent)
	}
	if req.XGoogTopic != "projects/demo/topics/test" {
		t.Fatalf("unexpected x-goog-topic: %q", req.XGoogTopic)
	}
	if req.XGoogDeliveryAttempt != "3" {
		t.Fatalf("unexpected x-goog-delivery-attempt: %q", req.XGoogDeliveryAttempt)
	}
	if req.Authorization != expectedAuthorization {
		t.Fatalf("unexpected authorization header: %q", req.Authorization)
	}
	if req.Token != "" {
		t.Fatalf("token must not be sent via query, got: %q", req.Token)
	}
}
