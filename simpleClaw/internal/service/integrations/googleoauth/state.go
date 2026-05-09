package googleoauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type StateCodec struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewStateCodec(secret []byte, ttl time.Duration, now func() time.Time) *StateCodec {
	if now == nil {
		now = time.Now
	}

	return &StateCodec{
		secret: append([]byte(nil), secret...),
		ttl:    ttl,
		now:    now,
	}
}

func (c *StateCodec) Sign(payload StatePayload) (string, error) {
	if len(c.secret) == 0 {
		return "", ErrStateSecretRequired
	}

	capabilities, err := NormalizeCapabilities(payload.Capabilities)
	if err != nil {
		return "", err
	}

	returnTo, err := normalizeReturnTo(payload.ReturnTo)
	if err != nil {
		return "", err
	}

	now := c.now().UTC()
	payload.Capabilities = capabilities
	payload.ReturnTo = returnTo
	payload.IssuedAt = now
	payload.ExpiresAt = now.Add(c.ttl)

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal state payload: %w", err)
	}

	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	signature := mac.Sum(nil)

	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (c *StateCodec) Parse(raw string) (StatePayload, error) {
	if len(c.secret) == 0 {
		return StatePayload{}, ErrStateSecretRequired
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return StatePayload{}, ErrStateInvalid
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return StatePayload{}, ErrStateInvalid
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return StatePayload{}, ErrStateInvalid
	}

	mac := hmac.New(sha256.New, c.secret)
	mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return StatePayload{}, ErrStateInvalid
	}

	var payload StatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return StatePayload{}, ErrStateInvalid
	}

	capabilities, err := NormalizeCapabilities(payload.Capabilities)
	if err != nil {
		return StatePayload{}, ErrStateInvalid
	}

	returnTo, err := normalizeReturnTo(payload.ReturnTo)
	if err != nil {
		return StatePayload{}, ErrStateInvalid
	}

	if c.now().UTC().After(payload.ExpiresAt) {
		return StatePayload{}, ErrStateExpired
	}

	payload.Capabilities = capabilities
	payload.ReturnTo = returnTo

	return payload, nil
}

func normalizeReturnTo(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	if !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", ErrReturnToInvalid
	}

	if strings.Contains(raw, "://") {
		return "", ErrReturnToInvalid
	}

	return raw, nil
}
