package googleoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

const defaultUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

type tokenClientFactory interface {
	Client(ctx context.Context, token *oauth2.Token) *http.Client
}

type HTTPProfileFetcher struct {
	clients     tokenClientFactory
	userInfoURL string
}

func NewHTTPProfileFetcher(clients tokenClientFactory) *HTTPProfileFetcher {
	return &HTTPProfileFetcher{
		clients:     clients,
		userInfoURL: defaultUserInfoURL,
	}
}

func (f *HTTPProfileFetcher) FetchProfile(ctx context.Context, token *oauth2.Token) (GoogleProfile, error) {
	if f.clients == nil || token == nil {
		return GoogleProfile{}, fmt.Errorf("profile fetcher is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.userInfoURL, nil)
	if err != nil {
		return GoogleProfile{}, err
	}

	resp, err := f.clients.Client(ctx, token).Do(req)
	if err != nil {
		return GoogleProfile{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return GoogleProfile{}, fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}

	var payload struct {
		ID      string `json:"id"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return GoogleProfile{}, err
	}

	return GoogleProfile{
		ID:      strings.TrimSpace(payload.ID),
		Email:   strings.TrimSpace(payload.Email),
		Name:    strings.TrimSpace(payload.Name),
		Picture: strings.TrimSpace(payload.Picture),
	}, nil
}
