package user

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"shared/pkg/jwt"
	"simpleClaw/internal/entities"
	"simpleClaw/internal/infra/sql"
	"simpleClaw/internal/service/user/commands"
)

func TestSignInCreatesAdminUserForConfiguredEmail(t *testing.T) {
	t.Parallel()

	storage := newFakeUserStorage()
	service := NewUser(
		newTestJWT(t),
		storage,
		fakeChannelStorage{},
		newFakePaymentMethodStorage(),
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
		newFakePaymentMethodStorage(),
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

func TestSignInWithoutAPIKeyManagerCreatesUserWithoutOpenRouterKey(t *testing.T) {
	t.Parallel()

	storage := newFakeUserStorage()
	service := NewUser(
		newTestJWT(t),
		storage,
		fakeChannelStorage{},
		newFakePaymentMethodStorage(),
		nil,
		nil,
	)

	_, _, err := service.SignIn(context.Background(), commands.SignIn{
		Name:  "User",
		Email: "user@example.com",
	})
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	user, err := storage.GetByEmail(context.Background(), "user@example.com")
	if err != nil {
		t.Fatalf("get stored user: %v", err)
	}

	if user.OpenRouterApiKey != "" {
		t.Fatalf("openrouter api key = %q, want empty", user.OpenRouterApiKey)
	}

	if user.OpenRouterKeyID != "" {
		t.Fatalf("openrouter key id = %q, want empty", user.OpenRouterKeyID)
	}
}

func TestPaymentMethodEntityExists(t *testing.T) {
	t.Parallel()

	method := entities.PaymentMethod{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Title:     "Primary card",
		IsDefault: true,
	}

	if method.Title == "" {
		t.Fatal("expected title")
	}
}

func TestGetPaymentMethodReturnsStoredMethod(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	methodID := uuid.New()
	pmStore := newFakePaymentMethodStorage()
	pmStore.methods[methodID] = entities.PaymentMethod{
		ID:        methodID,
		UserID:    userID,
		Title:     "Primary card",
		IsDefault: true,
	}

	service := NewUser(
		newTestJWT(t),
		newFakeUserStorage(),
		fakeChannelStorage{},
		pmStore,
		fakeAPIKeyManager{},
		nil,
	)

	method, err := service.GetPaymentMethod(context.Background(), methodID, userID)
	if err != nil {
		t.Fatalf("GetPaymentMethod() error = %v", err)
	}

	if method.ID != methodID {
		t.Fatalf("method id = %s, want %s", method.ID, methodID)
	}

	if method.Title != "Primary card" {
		t.Fatalf("method title = %q, want %q", method.Title, "Primary card")
	}
}

func TestGetPaymentMethodsListsUserMethods(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	pmStore := newFakePaymentMethodStorage()
	firstID := uuid.New()
	secondID := uuid.New()
	otherID := uuid.New()
	pmStore.methods[firstID] = entities.PaymentMethod{
		ID:        firstID,
		UserID:    userID,
		Title:     "First",
		IsDefault: true,
	}
	pmStore.methods[secondID] = entities.PaymentMethod{
		ID:        secondID,
		UserID:    userID,
		Title:     "Second",
		IsDefault: false,
	}
	pmStore.methods[otherID] = entities.PaymentMethod{
		ID:        otherID,
		UserID:    uuid.New(),
		Title:     "Other user",
		IsDefault: false,
	}

	service := NewUser(
		newTestJWT(t),
		newFakeUserStorage(),
		fakeChannelStorage{},
		pmStore,
		fakeAPIKeyManager{},
		nil,
	)

	methods, err := service.GetPaymentMethods(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetPaymentMethods() error = %v", err)
	}

	if len(methods) != 2 {
		t.Fatalf("methods len = %d, want 2", len(methods))
	}
}

func TestSetDefaultPaymentMethodSwitchesDefault(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	pmStore := newFakePaymentMethodStorage()
	service := NewUser(
		newTestJWT(t),
		newFakeUserStorage(),
		fakeChannelStorage{},
		pmStore,
		fakeAPIKeyManager{},
		nil,
	)

	first := entities.PaymentMethod{ID: uuid.New(), UserID: userID, Title: "First", IsDefault: true}
	second := entities.PaymentMethod{ID: uuid.New(), UserID: userID, Title: "Second", IsDefault: false}
	pmStore.methods[first.ID] = first
	pmStore.methods[second.ID] = second

	err := service.SetDefaultPaymentMethod(context.Background(), commands.SetDefaultPaymentMethod{
		UserID:          userID,
		PaymentMethodID: second.ID,
	})
	if err != nil {
		t.Fatalf("SetDefaultPaymentMethod() error = %v", err)
	}

	storedFirst, err := pmStore.GetByID(context.Background(), first.ID, userID)
	if err != nil {
		t.Fatalf("GetByID() first error = %v", err)
	}

	storedSecond, err := pmStore.GetByID(context.Background(), second.ID, userID)
	if err != nil {
		t.Fatalf("GetByID() second error = %v", err)
	}

	if storedFirst.IsDefault {
		t.Fatal("old default must be cleared")
	}

	if !storedSecond.IsDefault {
		t.Fatal("selected method must become default")
	}
}

func TestRemovePaymentMethodDeletesOwnerScopedMethod(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	pmStore := newFakePaymentMethodStorage()
	service := NewUser(
		newTestJWT(t),
		newFakeUserStorage(),
		fakeChannelStorage{},
		pmStore,
		fakeAPIKeyManager{},
		nil,
	)

	method := entities.PaymentMethod{ID: uuid.New(), UserID: userID, Title: "Primary card", IsDefault: true}
	pmStore.methods[method.ID] = method

	if err := service.RemovePaymentMethod(context.Background(), method.ID, userID); err != nil {
		t.Fatalf("RemovePaymentMethod() error = %v", err)
	}

	_, err := pmStore.GetByID(context.Background(), method.ID, userID)
	if !errors.Is(err, sql.ErrNotFound) {
		t.Fatalf("GetByID() after delete error = %v, want ErrNotFound", err)
	}
}

func TestRefreshDeletesSessionWhenRefreshTokenExpired(t *testing.T) {
	t.Parallel()

	j := newTestJWTWithRefreshExpiry(t, -time.Minute)
	storage := newFakeUserStorage()
	service := NewUser(j, storage, fakeChannelStorage{}, newFakePaymentMethodStorage(), nil, nil)

	userID := uuid.New()
	sessionID := uuid.New()
	_, refresh, err := j.GeneratePair(userID, sessionID)
	if err != nil {
		t.Fatalf("GeneratePair() error = %v", err)
	}

	storage.sessions[sessionID] = entities.Session{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: refresh.Raw,
		CreatedAt:    time.Now(),
	}

	_, _, err = service.Refresh(context.Background(), refresh.Raw)
	if !errors.Is(err, jwt.ErrExpired) {
		t.Fatalf("Refresh() error = %v, want ErrExpired", err)
	}

	if _, ok := storage.sessions[sessionID]; ok {
		t.Fatal("expired refresh session must be deleted")
	}
}

func TestRefreshRotatesStoredRefreshToken(t *testing.T) {
	t.Parallel()

	j := newTestJWT(t)
	storage := newFakeUserStorage()
	service := NewUser(j, storage, fakeChannelStorage{}, newFakePaymentMethodStorage(), nil, nil)

	userID := uuid.New()
	sessionID := uuid.New()
	_, refresh, err := j.GeneratePair(userID, sessionID)
	if err != nil {
		t.Fatalf("GeneratePair() error = %v", err)
	}

	storage.sessions[sessionID] = entities.Session{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: refresh.Raw,
		CreatedAt:    time.Now(),
	}

	_, rotated, err := service.Refresh(context.Background(), refresh.Raw)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	stored, err := storage.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}

	if stored.RefreshToken != rotated.Raw {
		t.Fatalf("stored refresh token = %q, want %q", stored.RefreshToken, rotated.Raw)
	}
}

func TestLogoutDeletesSession(t *testing.T) {
	t.Parallel()

	storage := newFakeUserStorage()
	service := NewUser(newTestJWT(t), storage, fakeChannelStorage{}, newFakePaymentMethodStorage(), nil, nil)

	userID := uuid.New()
	sessionID := uuid.New()
	storage.sessions[sessionID] = entities.Session{
		ID:           sessionID,
		UserID:       userID,
		RefreshToken: "refresh-token",
		CreatedAt:    time.Now(),
	}

	err := service.Logout(context.Background(), commands.Logout{
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	if _, ok := storage.sessions[sessionID]; ok {
		t.Fatal("session must be deleted on sign out")
	}
}

func newTestJWT(t *testing.T) jwt.JWT {
	t.Helper()

	return newTestJWTWithRefreshExpiry(t, 24*time.Hour)
}

func newTestJWTWithRefreshExpiry(t *testing.T, refreshExpiry time.Duration) jwt.JWT {
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

	return jwt.New(privatePEM, publicPEM, []byte("test-refresh-secret"), time.Hour, refreshExpiry)
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

type fakePaymentStorage struct{}

func (fakePaymentStorage) Create(context.Context, entities.Payment) error { return nil }

func (fakePaymentStorage) GetByID(
	context.Context,
	string,
	uuid.UUID,
) (entities.Payment, error) {
	return entities.Payment{}, sql.ErrNotFound
}

func (fakePaymentStorage) GetByUserID(context.Context, uuid.UUID) ([]entities.Payment, error) {
	return nil, nil
}

func (fakePaymentStorage) Update(context.Context, entities.Payment) error { return nil }

func (fakePaymentStorage) Delete(context.Context, string, uuid.UUID) error { return nil }

type fakePaymentMethodStorage struct {
	methods map[uuid.UUID]entities.PaymentMethod
}

func newFakePaymentMethodStorage() *fakePaymentMethodStorage {
	return &fakePaymentMethodStorage{
		methods: map[uuid.UUID]entities.PaymentMethod{},
	}
}

func (s *fakePaymentMethodStorage) Create(_ context.Context, method entities.PaymentMethod) error {
	s.methods[method.ID] = method
	return nil
}

func (s *fakePaymentMethodStorage) GetByID(
	_ context.Context,
	id, userID uuid.UUID,
) (entities.PaymentMethod, error) {
	method, ok := s.methods[id]
	if !ok || method.UserID != userID {
		return entities.PaymentMethod{}, sql.ErrNotFound
	}

	return method, nil
}

func (s *fakePaymentMethodStorage) GetByUserID(
	_ context.Context,
	userID uuid.UUID,
) ([]entities.PaymentMethod, error) {
	methods := make([]entities.PaymentMethod, 0, len(s.methods))
	for _, method := range s.methods {
		if method.UserID == userID {
			methods = append(methods, method)
		}
	}

	return methods, nil
}

func (s *fakePaymentMethodStorage) UpdateDefault(
	_ context.Context,
	id, userID uuid.UUID,
	isDefault bool,
) error {
	method, ok := s.methods[id]
	if !ok || method.UserID != userID {
		return sql.ErrNotFound
	}

	method.IsDefault = isDefault
	s.methods[id] = method
	return nil
}

func (s *fakePaymentMethodStorage) Delete(_ context.Context, id, userID uuid.UUID) error {
	method, ok := s.methods[id]
	if !ok || method.UserID != userID {
		return sql.ErrNotFound
	}

	delete(s.methods, id)
	return nil
}

func (s *fakePaymentMethodStorage) ClearDefaultByUserID(_ context.Context, userID uuid.UUID) error {
	for id, method := range s.methods {
		if method.UserID != userID {
			continue
		}

		method.IsDefault = false
		s.methods[id] = method
	}

	return nil
}

func (s *fakePaymentMethodStorage) CountByUserID(_ context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	for _, method := range s.methods {
		if method.UserID == userID {
			count++
		}
	}

	return count, nil
}

type fakeAPIKeyManager struct{}

func (fakeAPIKeyManager) Create(
	context.Context,
	uuid.UUID,
	float64,
) (entities.OpenRouterKey, error) {
	return entities.OpenRouterKey{
		ID:     "test-key-id",
		Secret: "test-secret",
	}, nil
}
