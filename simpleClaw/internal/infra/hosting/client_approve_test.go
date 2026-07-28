package hosting

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientApprovePairing_InvalidCode(t *testing.T) {
	t.Parallel()

	c := &client{
		http: &http.Client{
			Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Status:     "400 Bad Request",
					Body:       io.NopCloser(strings.NewReader(`{"code":"invalid_code","message":"invalid code"}`)),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	err := c.approvePairing(
		context.Background(),
		"http://container-manager.local",
		nil,
		"user-1",
		"claw-1",
		"telegram",
		"wrong-code",
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got: %v", err)
	}
}

func TestClientApprovePairing_SendsChannelType(t *testing.T) {
	t.Parallel()

	c := &client{
		http: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.URL.Query().Get("channelType"); got != "telegram" {
					t.Fatalf("channelType = %q, want %q", got, "telegram")
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     make(http.Header),
				}, nil
			}),
		},
	}

	if err := c.approvePairing(
		context.Background(),
		"http://container-manager.local",
		nil,
		"user-1",
		"claw-1",
		"telegram",
		"123456",
	); err != nil {
		t.Fatalf("approvePairing() error = %v", err)
	}
}
