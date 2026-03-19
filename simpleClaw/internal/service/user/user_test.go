package user

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"shared/pkg/jwt"

	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/user/commands"

	"github.com/google/uuid"
)

func TestSignInCreatesAdminUserForConfiguredEmail(t *testing.T) {
	t.Parallel()

	storage := newFakeUserStorage()
	service := NewUser(
		newTestJWT(t),
		storage,
		fakeChannelStorage{},
		fakeAPIKeyManager{},
		[]string{"admin@example.com"},
	)

	_, _, err := service.SignIn(context.Background(), commands.SignIn{
		Name:      "Admin",
		NickName:  "admin",
		AvatarURL: "http://example.com/avatar.png",
		Email:     " Admin@Example.com ",
	})
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	user, err := storage.GetByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("get stored user: %v", err)
	}

	if user.Role != entities.AdminRole {
		t.Fatalf("expected admin role, got %q", user.Role)
	}
}

func TestSignInUpdatesRoleForExistingConfiguredAdmin(t *testing.T) {
	t.Parallel()

	storage := newFakeUserStorage()
	existing := entities.NewUser("User", "user", "", "admin@example.com", entities.UserRole)
	if err := storage.Create(context.Background(), existing); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	service := NewUser(
		newTestJWT(t),
		storage,
		fakeChannelStorage{},
		fakeAPIKeyManager{},
		[]string{"admin@example.com"},
	)

	_, _, err := service.SignIn(context.Background(), commands.SignIn{
		Name:  "User",
		Email: "admin@example.com",
	})
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	user, err := storage.GetByID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("get updated user: %v", err)
	}

	if user.Role != entities.AdminRole {
		t.Fatalf("expected admin role after sign in, got %q", user.Role)
	}
}

func newTestJWT(t *testing.T) jwt.JWT {
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

	return jwt.New(privatePEM, publicPEM, []byte("test-refresh-secret"), time.Hour, 24*time.Hour)
}

type fakeUserStorage struct {
	users    map[uuid.UUID]entities.User
	sessions map[uuid.UUID]entities.Session
}

func newFakeUserStorage() *fakeUserStorage {
	return &fakeUserStorage{
		users:    map[uuid.UUID]entities.User{},
		sessions: map[uuid.UUID]entities.Session{},
	}
}

func (s *fakeUserStorage) Create(_ context.Context, u entities.User) error {
	s.users[u.ID] = u
	return nil
}

func (s *fakeUserStorage) Delete(_ context.Context, id uuid.UUID) error {
	delete(s.users, id)
	return nil
}

func (s *fakeUserStorage) CreateSession(_ context.Context, session entities.Session) error {
	s.sessions[session.ID] = session
	return nil
}

func (s *fakeUserStorage) DeleteSession(_ context.Context, session entities.Session) error {
	delete(s.sessions, session.ID)
	return nil
}

func (s *fakeUserStorage) GetSessions(_ context.Context, userID uuid.UUID) ([]entities.Session, error) {
	result := make([]entities.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session.UserID == userID {
			result = append(result, session)
		}
	}

	return result, nil
}

func (s *fakeUserStorage) GetSession(_ context.Context, id uuid.UUID) (entities.Session, error) {
	session, ok := s.sessions[id]
	if !ok {
		return entities.Session{}, sql.ErrNotFound
	}

	return session, nil
}

func (s *fakeUserStorage) UpdateSessionRefresh(
	_ context.Context,
	sessionID, _ uuid.UUID,
	refreshToken string,
) error {
	session, ok := s.sessions[sessionID]
	if !ok {
		return sql.ErrNotFound
	}

	session.RefreshToken = refreshToken
	s.sessions[sessionID] = session
	return nil
}

func (s *fakeUserStorage) GetByEmail(_ context.Context, email string) (entities.User, error) {
	email = normalizeEmail(email)

	for _, user := range s.users {
		if normalizeEmail(user.Email) == email {
			return user, nil
		}
	}

	return entities.User{}, sql.ErrNotFound
}

func (s *fakeUserStorage) GetByID(_ context.Context, id uuid.UUID) (entities.User, error) {
	user, ok := s.users[id]
	if !ok {
		return entities.User{}, sql.ErrNotFound
	}

	return user, nil
}

func (s *fakeUserStorage) UpdateRole(_ context.Context, id uuid.UUID, role string) error {
	user, ok := s.users[id]
	if !ok {
		return sql.ErrNotFound
	}

	user.Role = role
	s.users[id] = user
	return nil
}

func (s *fakeUserStorage) UpdateOpenRouterKey(
	_ context.Context,
	id uuid.UUID,
	key entities.OpenRouterKey,
) error {
	user, ok := s.users[id]
	if !ok {
		return sql.ErrNotFound
	}

	user.OpenRouterKeyID = key.ID
	user.OpenRouterApiKey = key.Secret
	s.users[id] = user
	return nil
}

func (s *fakeUserStorage) GetGmailToken(_ context.Context, _ uuid.UUID) (entities.GmailToken, error) {
	return entities.GmailToken{}, sql.ErrNotFound
}

func (s *fakeUserStorage) UpsertGmailToken(
	_ context.Context,
	_ uuid.UUID,
	_ entities.GmailToken,
) error {
	return nil
}

type fakeChannelStorage struct{}

func (fakeChannelStorage) Create(context.Context, entities.Channel) error { return nil }

func (fakeChannelStorage) GetByID(context.Context, uuid.UUID, uuid.UUID) (entities.Channel, error) {
	return entities.Channel{}, sql.ErrNotFound
}

func (fakeChannelStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Channel, error) {
	return nil, nil
}

func (fakeChannelStorage) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (fakeChannelStorage) Update(context.Context, entities.Channel) error { return nil }

type fakeAPIKeyManager struct{}

func (fakeAPIKeyManager) Create(
	context.Context,
	uuid.UUID,
	string,
	float64,
) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{
		ID:     "test-key-id",
		Secret: "test-secret",
	}, nil
}
