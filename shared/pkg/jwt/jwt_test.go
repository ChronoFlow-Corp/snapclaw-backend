package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestParseRefreshReturnsExpiredForExpiredToken(t *testing.T) {
	t.Parallel()

	j := newTestJWT(t, "primary-secret", -time.Minute)
	_, refresh, err := j.GeneratePair(newUUID(t), newUUID(t))
	if err != nil {
		t.Fatalf("GeneratePair() error = %v", err)
	}

	_, err = j.ParseRefresh(refresh.Raw)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("ParseRefresh() error = %v, want ErrExpired", err)
	}
}

func TestParseRefreshAllowExpiredReturnsClaimsForExpiredToken(t *testing.T) {
	t.Parallel()

	userID := newUUID(t)
	sessionID := newUUID(t)
	j := newTestJWT(t, "primary-secret", -time.Minute)
	_, refresh, err := j.GeneratePair(userID, sessionID)
	if err != nil {
		t.Fatalf("GeneratePair() error = %v", err)
	}

	parsed, err := j.ParseRefreshAllowExpired(refresh.Raw)
	if err != nil {
		t.Fatalf("ParseRefreshAllowExpired() error = %v", err)
	}

	if parsed.Claims.UserID != userID {
		t.Fatalf("user id = %s, want %s", parsed.Claims.UserID, userID)
	}

	if parsed.Claims.SessionID != sessionID {
		t.Fatalf("session id = %s, want %s", parsed.Claims.SessionID, sessionID)
	}
}

func TestParseRefreshAllowExpiredRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	issuer := newTestJWT(t, "issuer-secret", -time.Minute)
	consumer := newTestJWT(t, "consumer-secret", -time.Minute)
	_, refresh, err := issuer.GeneratePair(newUUID(t), newUUID(t))
	if err != nil {
		t.Fatalf("GeneratePair() error = %v", err)
	}

	_, err = consumer.ParseRefreshAllowExpired(refresh.Raw)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("ParseRefreshAllowExpired() error = %v, want ErrInvalid", err)
	}
}

func newTestJWT(t *testing.T, refreshSecret string, refreshExpires time.Duration) JWT {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	privatePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey),
	})

	return New(privatePEM, publicPEM, []byte(refreshSecret), time.Hour, refreshExpires)
}

func newUUID(t *testing.T) uuid.UUID {
	t.Helper()

	id, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("uuid.NewRandom() error = %v", err)
	}

	return id
}
