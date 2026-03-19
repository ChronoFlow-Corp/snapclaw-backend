package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	errGogConnectPayloadRequired = errors.New("connect payload is required")
	errGogEmailRequired          = errors.New("email is required")
	errGogRefreshTokenRequired   = errors.New("refresh token is required")
	errGogWatchTopicRequired     = errors.New("watch topic is required")
)

type gogImportPayload struct {
	Email        string   `json:"email"`
	Client       string   `json:"client,omitempty"`
	RefreshToken string   `json:"refresh_token"`
	Topic        string   `json:"topic"`
	Labels       []string `json:"labels,omitempty"`
}

type legacyGmailPayload struct {
	Email  string   `json:"email"`
	Client string   `json:"client"`
	Topic  string   `json:"topic"`
	Labels []string `json:"labels"`
	Token  struct {
		RefreshToken string `json:"refresh_token"`
	} `json:"token"`
}

func normalizeGogImportPayload(raw []byte) ([]byte, error) {
	payload := bytes.TrimSpace(raw)
	if len(payload) == 0 {
		return nil, errGogConnectPayloadRequired
	}

	if direct, ok := parseGogImportPayload(payload); ok {
		return json.Marshal(direct)
	}

	legacy, err := parseLegacyGmailPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("invalid connect payload: %w", err)
	}

	return json.Marshal(legacy)
}

func parseGogImportPayload(raw []byte) (gogImportPayload, bool) {
	var direct gogImportPayload
	if err := json.Unmarshal(raw, &direct); err != nil {
		return gogImportPayload{}, false
	}

	if strings.TrimSpace(direct.RefreshToken) == "" {
		return gogImportPayload{}, false
	}

	normalized, err := normalizeImportPayload(
		direct.Email,
		direct.Client,
		direct.RefreshToken,
		direct.Topic,
		direct.Labels,
	)
	if err != nil {
		return gogImportPayload{}, false
	}

	return normalized, true
}

func parseLegacyGmailPayload(raw []byte) (gogImportPayload, error) {
	var legacy legacyGmailPayload
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return gogImportPayload{}, err
	}

	return normalizeImportPayload(
		legacy.Email,
		legacy.Client,
		legacy.Token.RefreshToken,
		legacy.Topic,
		legacy.Labels,
	)
}

func normalizeImportPayload(
	email,
	client,
	refreshToken,
	topic string,
	labels []string,
) (gogImportPayload, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return gogImportPayload{}, errGogEmailRequired
	}

	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return gogImportPayload{}, errGogRefreshTokenRequired
	}

	client = strings.TrimSpace(client)
	if client == "" {
		client = "default"
	}

	topic = strings.TrimSpace(topic)
	if topic == "" {
		return gogImportPayload{}, errGogWatchTopicRequired
	}

	return gogImportPayload{
		Email:        email,
		Client:       client,
		RefreshToken: refreshToken,
		Topic:        topic,
		Labels:       normalizeGogWatchLabels(labels),
	}, nil
}

func normalizeGogWatchLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(labels))
	out := make([]string, 0, len(labels))
	for _, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}

		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		out = append(out, label)
	}

	return out
}
