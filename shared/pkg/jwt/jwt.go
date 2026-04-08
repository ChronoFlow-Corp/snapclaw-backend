package jwt

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type JWT struct {
	accessSecretPrivate []byte
	accessSecretPublic  []byte
	refreshSecret       []byte
	accessExpires       time.Duration
	refreshExpires      time.Duration
}
type Token struct {
	AccessToken string `json:"access_token"`
}

type AccessToken struct {
	Raw    string `json:"raw_token"`
	Claims accessClaims
}

type RefreshToken struct {
	Raw    string `json:"raw_token"`
	Claims refreshClaims
}

type refreshClaims struct {
	jwt.RegisteredClaims

	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
}

type accessClaims struct {
	jwt.RegisteredClaims

	SessionID uuid.UUID `json:"session_id"`
	UserID    uuid.UUID `json:"user_id"`
}

// New creates JWT client.
func New(accessSecretPrivate, accessSecretPublic, refreshSecret []byte,
	accessExpires, refreshExpires time.Duration,
) JWT {
	return JWT{
		accessSecretPrivate: accessSecretPrivate,
		accessSecretPublic:  accessSecretPublic,
		refreshSecret:       refreshSecret,
		accessExpires:       accessExpires,
		refreshExpires:      refreshExpires,
	}
}

// NewPair generate new pair jwt tokens.
func (j JWT) GeneratePair(
	userID, sessionID uuid.UUID,
) (access AccessToken, refresh RefreshToken, err error) {
	const op = "pkg.jwt.NewPair"

	access, err = j.newAccess(userID, sessionID)
	if err != nil {
		return AccessToken{}, RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	refresh, err = j.newRefresh(userID, sessionID)
	if err != nil {
		return AccessToken{}, RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return access, refresh, nil
}

// ParseAccess parse raw token and return claims.
func (j JWT) ParseAccess(raw string, f any) (AccessToken, error) {
	var key any
	switch parseFunc := f.(type) {
	case func(key []byte) (*rsa.PrivateKey, error):
		var err error

		key, err = parseFunc(j.accessSecretPrivate)
		if err != nil {
			return AccessToken{}, err
		}
	case func(key []byte) (*rsa.PublicKey, error):
		var err error

		key, err = parseFunc(j.accessSecretPublic)
		if err != nil {
			return AccessToken{}, err
		}
	default:
		return AccessToken{}, ErrInvalidParseFunc
	}

	var cl accessClaims

	_, err := jwt.ParseWithClaims(raw, &cl, func(_ *jwt.Token) (any, error) {
		return key, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return AccessToken{}, fmt.Errorf("%w: %w", ErrExpired, err)
		}

		return AccessToken{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return AccessToken{Raw: raw, Claims: cl}, nil
}

// ParseRefresh parse raw token and return claims.
func (j JWT) ParseRefresh(raw string) (RefreshToken, error) {
	var cl refreshClaims

	_, err := jwt.ParseWithClaims(raw, &cl, func(_ *jwt.Token) (any, error) {
		return j.refreshSecret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return RefreshToken{}, fmt.Errorf("%w: %w", ErrExpired, err)
		}

		return RefreshToken{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return RefreshToken{Raw: raw, Claims: cl}, nil
}

// ParseRefreshAllowExpired parses refresh token claims while still verifying the signature.
// It intentionally skips claims validation so expired tokens can be mapped back to a stored session.
func (j JWT) ParseRefreshAllowExpired(raw string) (RefreshToken, error) {
	var cl refreshClaims

	_, err := jwt.ParseWithClaims(raw, &cl, func(_ *jwt.Token) (any, error) {
		return j.refreshSecret, nil
	}, jwt.WithoutClaimsValidation())
	if err != nil {
		return RefreshToken{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}

	return RefreshToken{Raw: raw, Claims: cl}, nil
}

func (j JWT) newRefresh(userID, sessionID uuid.UUID) (RefreshToken, error) {
	const op = "jwt.newRefresh"

	claims := refreshClaims{
		SessionID: sessionID,
		UserID:    userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.refreshExpires)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)

	raw, err := t.SignedString(j.refreshSecret)
	if err != nil {
		return RefreshToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return RefreshToken{Raw: raw, Claims: claims}, nil
}

func (j JWT) newAccess(userID, sessionID uuid.UUID) (AccessToken, error) {
	const op = "jwt.newAccess"

	key, err := jwt.ParseRSAPrivateKeyFromPEM(j.accessSecretPrivate)
	if err != nil {
		return AccessToken{}, fmt.Errorf("%s: %w", op, err)
	}

	claims := accessClaims{
		SessionID: sessionID,
		UserID:    userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    userID.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.accessExpires)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS512, claims)

	raw, err := t.SignedString(key)
	if err != nil {
		return AccessToken{}, fmt.Errorf("%s: %w", op, err)
	}

	return AccessToken{Raw: raw, Claims: claims}, nil
}
