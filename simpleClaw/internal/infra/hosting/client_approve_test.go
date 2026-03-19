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
					Body:       io.NopCloser(strings.NewReader("invalid code\n")),
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
		"wrong-code",
	)
	if err == nil {
		t.Fatal("expected error")
	}

	if !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expected ErrInvalidCode, got: %v", err)
	}
}
