package controllers_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"shared/pkg/jwt"
	"sort"
	"testing"
	"time"

	"simpleClaw/config"
	"simpleClaw/internal/api/rest/controllers"
	"simpleClaw/internal/entities"
	entitychannels "simpleClaw/internal/entities/channels"
	"simpleClaw/internal/infra/sql"
	clawcommands "simpleClaw/internal/service/claw/commands"
	usercommands "simpleClaw/internal/service/user/commands"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type testEnv struct {
	router       http.Handler
	user         entities.User
	sessionID    uuid.UUID
	accessToken  string
	refreshToken string
	userService  *fakeUserService
	clawService  *fakeClawService
}

func TestRoutesIntegration(t *testing.T) {
	t.Run("POST /api/auth/refresh", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodPost,
			"/api/auth/refresh",
			nil,
			refreshCookie(env.refreshToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)

		if !payload["refreshed"] {
			t.Fatalf("expected refreshed=true, got payload=%v", payload)
		}

		cookies := rr.Result().Cookies()
		if !hasCookie(cookies, "access_token") {
			t.Fatalf("expected access_token cookie in response")
		}
		if !hasCookie(cookies, "refresh_token") {
			t.Fatalf("expected refresh_token cookie in response")
		}
	})

	t.Run("GET /api/auth/user-info", func(t *testing.T) {
		env := newTestEnv(t)

		rr := env.request(
			t,
			http.MethodGet,
			"/api/auth/user-info",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != env.user.ID.String() {
			t.Fatalf("unexpected user id: %s", payload.ID)
		}
		if payload.Email != env.user.Email {
			t.Fatalf("unexpected email: %s", payload.Email)
		}
	})

	t.Run("POST /api/channel", func(t *testing.T) {
		env := newTestEnv(t)

		body := map[string]any{
			"name": "alerts",
			"telegramChannel": map[string]any{
				"dmPolicy":  "allowlist",
				"botToken":  "test-bot-token",
				"allowFrom": []string{"1001", "1002"},
			},
		}

		rr := env.request(t, http.MethodPost, "/api/channel", body, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID          string `json:"ID"`
			Name        string `json:"Name"`
			ChannelType string `json:"ChannelType"`
			UserID      string `json:"UserID"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatalf("expected non-empty channel ID")
		}
		if payload.Name != "alerts" {
			t.Fatalf("unexpected channel name: %s", payload.Name)
		}
		if payload.ChannelType != entities.ChannelTelegramType {
			t.Fatalf("unexpected channel type: %s", payload.ChannelType)
		}
		if payload.UserID != env.user.ID.String() {
			t.Fatalf("unexpected user ID: %s", payload.UserID)
		}
	})

	t.Run("GET /api/claws", func(t *testing.T) {
		env := newTestEnv(t)
		env.clawService.seed(env.user.ID, "first")
		env.clawService.seed(env.user.ID, "second")

		rr := env.request(t, http.MethodGet, "/api/claws", nil, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload []struct {
			ID   string `json:"ID"`
			Name string `json:"Name"`
		}
		decodeJSON(t, rr, &payload)

		if len(payload) != 2 {
			t.Fatalf("expected 2 claws, got %d", len(payload))
		}
	})

	t.Run("GET /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "single")

		rr := env.request(
			t,
			http.MethodGet,
			"/api/claws/"+cl.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID   string `json:"ID"`
			Name string `json:"Name"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}
		if payload.Name != "single" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}
	})

	t.Run("POST /api/claws", func(t *testing.T) {
		env := newTestEnv(t)

		body := map[string]any{
			"name":       "new-claw",
			"model":      "openai/gpt-4.1-mini",
			"channelIds": []string{uuid.NewString()},
			"apiLimits": map[string]any{
				"requestsPerMinute": 120,
				"monthlyBudgetUsd":  15.5,
			},
		}

		rr := env.request(t, http.MethodPost, "/api/claws", body, accessCookie(env.accessToken))

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			UserID string `json:"UserID"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID == "" {
			t.Fatalf("expected non-empty claw ID")
		}
		if payload.Name != "new-claw" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}
		if payload.UserID != env.user.ID.String() {
			t.Fatalf("unexpected user id: %s", payload.UserID)
		}
	})

	t.Run("PUT /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "before-update")

		body := map[string]any{
			"name":       "after-update",
			"model":      "openai/gpt-4.1-mini",
			"channelIds": []string{uuid.NewString()},
			"apiLimits": map[string]any{
				"requestsPerMinute": 90,
				"monthlyBudgetUsd":  9.99,
			},
		}

		rr := env.request(
			t,
			http.MethodPut,
			"/api/claws/"+cl.ID.String(),
			body,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload struct {
			ID   string `json:"ID"`
			Name string `json:"Name"`
		}
		decodeJSON(t, rr, &payload)

		if payload.ID != cl.ID.String() {
			t.Fatalf("unexpected claw id: %s", payload.ID)
		}
		if payload.Name != "after-update" {
			t.Fatalf("unexpected claw name: %s", payload.Name)
		}
	})

	t.Run("POST /api/claws/{id}/start", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-start")

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/start",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)
		if !payload["started"] {
			t.Fatalf("expected started=true, got payload=%v", payload)
		}

		updated, err := env.clawService.GetByID(context.Background(), cl.ID, env.user.ID)
		if err != nil {
			t.Fatalf("get claw after start: %v", err)
		}

		if updated.Status != entities.StatusRunning {
			t.Fatalf("unexpected status after start: %s", updated.Status)
		}
	})

	t.Run("POST /api/claws/{id}/stop", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-stop")
		cl.Status = entities.StatusRunning
		env.clawService.claws[cl.ID] = cl

		rr := env.request(
			t,
			http.MethodPost,
			"/api/claws/"+cl.ID.String()+"/stop",
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)
		if !payload["stopped"] {
			t.Fatalf("expected stopped=true, got payload=%v", payload)
		}

		updated, err := env.clawService.GetByID(context.Background(), cl.ID, env.user.ID)
		if err != nil {
			t.Fatalf("get claw after stop: %v", err)
		}

		if updated.Status != entities.StatusStop {
			t.Fatalf("unexpected status after stop: %s", updated.Status)
		}
	})

	t.Run("DELETE /api/claws/{id}", func(t *testing.T) {
		env := newTestEnv(t)
		cl := env.clawService.seed(env.user.ID, "to-delete")

		rr := env.request(
			t,
			http.MethodDelete,
			"/api/claws/"+cl.ID.String(),
			nil,
			accessCookie(env.accessToken),
		)

		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}

		var payload map[string]bool
		decodeJSON(t, rr, &payload)

		if !payload["deleted"] {
			t.Fatalf("expected deleted=true, got payload=%v", payload)
		}
		if env.clawService.exists(cl.ID) {
			t.Fatalf("claw %s should be deleted", cl.ID)
		}
	})
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	j := newTestJWT(t)

	user := entities.NewUser("integration-user", "integration@example.com", entities.UserRole)
	sessionID := uuid.New()

	access, refresh, err := j.GeneratePair(user.ID, sessionID)
	if err != nil {
		t.Fatalf("generate token pair: %v", err)
	}

	uService := newFakeUserService(user, j, sessionID, refresh.Raw)
	cService := newFakeClawService()

	api := chi.NewRouter()
	controllers.NewUser(config.EnvDevelopment, uService, j).Register(api)
	controllers.NewClaw(cService, j).Register(api)

	root := chi.NewRouter()
	root.Mount("/api", api)

	return &testEnv{
		router:       root,
		user:         user,
		sessionID:    sessionID,
		accessToken:  access.Raw,
		refreshToken: refresh.Raw,
		userService:  uService,
		clawService:  cService,
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

func (e *testEnv) request(
	t *testing.T,
	method, path string,
	body any,
	cookies ...*http.Cookie,
) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody []byte
	var err error

	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}

	rr := httptest.NewRecorder()
	e.router.ServeHTTP(rr, req)

	return rr
}

func decodeJSON(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()

	if err := json.NewDecoder(rr.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rr.Body.String())
	}
}

func accessCookie(token string) *http.Cookie {
	return &http.Cookie{Name: "access_token", Value: token}
}

func refreshCookie(token string) *http.Cookie {
	return &http.Cookie{Name: "refresh_token", Value: token}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, c := range cookies {
		if c.Name == name && c.Value != "" {
			return true
		}
	}

	return false
}

type fakeUserService struct {
	j            jwt.JWT
	sessionID    uuid.UUID
	users        map[uuid.UUID]entities.User
	channels     map[uuid.UUID]entities.Channel
	lastRefresh  map[uuid.UUID]string
	channelOrder []uuid.UUID
}

func newFakeUserService(
	user entities.User,
	j jwt.JWT,
	sessionID uuid.UUID,
	refreshToken string,
) *fakeUserService {
	return &fakeUserService{
		j:           j,
		sessionID:   sessionID,
		users:       map[uuid.UUID]entities.User{user.ID: user},
		channels:    map[uuid.UUID]entities.Channel{},
		lastRefresh: map[uuid.UUID]string{user.ID: refreshToken},
	}
}

func (s *fakeUserService) SignIn(
	_ context.Context,
	_ usercommands.SignIn,
) (jwt.AccessToken, jwt.RefreshToken, error) {
	return jwt.AccessToken{}, jwt.RefreshToken{}, errors.New(
		"oauth routes are not covered in this suite",
	)
}

func (s *fakeUserService) Refresh(
	_ context.Context,
	rawRefresh string,
) (jwt.AccessToken, jwt.RefreshToken, error) {
	token, err := s.j.ParseRefresh(rawRefresh)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, err
	}

	expectedRefresh, ok := s.lastRefresh[token.Claims.UserID]
	if !ok || expectedRefresh != rawRefresh || token.Claims.SessionID != s.sessionID {
		return jwt.AccessToken{}, jwt.RefreshToken{}, jwt.ErrInvalid
	}

	access, refresh, err := s.j.GeneratePair(token.Claims.UserID, token.Claims.SessionID)
	if err != nil {
		return jwt.AccessToken{}, jwt.RefreshToken{}, err
	}

	s.lastRefresh[token.Claims.UserID] = refresh.Raw

	return access, refresh, nil
}

func (s *fakeUserService) UserInfo(_ context.Context, userID uuid.UUID) (entities.User, error) {
	user, ok := s.users[userID]
	if !ok {
		return entities.User{}, sql.ErrNotFound
	}

	return user, nil
}

func (s *fakeUserService) AddChannel(
	_ context.Context,
	cm usercommands.AddChannel,
) (entities.Channel, error) {
	if _, ok := s.users[cm.UserID]; !ok {
		return entities.Channel{}, sql.ErrNotFound
	}

	cfg := entities.ClawChannels{}
	if cm.Telegram != nil {
		cfg.Telegram = &entitychannels.TelegramConfig{
			DmPolicy:  entitychannels.DmPolicy(cm.Telegram.DmPolicy),
			AllowFrom: append([]string(nil), cm.Telegram.AllowFrom...),
			Enabled:   true,
			BotToken:  cm.Telegram.BotToken,
		}
	}

	ch := entities.NewChannel(entities.ChannelTelegramType, cm.Name, cfg, cm.UserID)
	s.channels[ch.ID] = ch
	s.channelOrder = append(s.channelOrder, ch.ID)

	return ch, nil
}

type fakeClawService struct {
	claws map[uuid.UUID]entities.Claw
}

func newFakeClawService() *fakeClawService {
	return &fakeClawService{
		claws: map[uuid.UUID]entities.Claw{},
	}
}

func (s *fakeClawService) seed(userID uuid.UUID, name string) entities.Claw {
	now := time.Now()
	cl := entities.Claw{
		ID:        uuid.New(),
		Name:      name,
		UserID:    userID,
		Status:    entities.StatusStop,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.claws[cl.ID] = cl

	return cl
}

func (s *fakeClawService) exists(id uuid.UUID) bool {
	_, ok := s.claws[id]

	return ok
}

func (s *fakeClawService) Create(
	_ context.Context,
	cm clawcommands.CreateClaw,
) (entities.Claw, error) {
	now := time.Now()
	cl := entities.Claw{
		ID:        uuid.New(),
		Name:      cm.Name,
		UserID:    cm.UserID,
		Status:    entities.StatusStop,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.claws[cl.ID] = cl

	return cl, nil
}

func (s *fakeClawService) GetByID(
	_ context.Context,
	clawID uuid.UUID,
	userID uuid.UUID,
) (entities.Claw, error) {
	cl, ok := s.claws[clawID]
	if !ok || cl.UserID != userID {
		return entities.Claw{}, sql.ErrNotFound
	}

	return cl, nil
}

func (s *fakeClawService) GetByUserID(
	_ context.Context,
	userID uuid.UUID,
) ([]entities.Claw, error) {
	result := make([]entities.Claw, 0, len(s.claws))
	for _, cl := range s.claws {
		if cl.UserID == userID {
			result = append(result, cl)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID.String() < result[j].ID.String()
	})

	return result, nil
}

func (s *fakeClawService) Update(
	_ context.Context,
	cm clawcommands.UpdateClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}

	existing.Name = cm.Name
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Start(
	_ context.Context,
	cm clawcommands.StartClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}

	existing.Status = entities.StatusRunning
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Stop(
	_ context.Context,
	cm clawcommands.StopClaw,
) (entities.Claw, error) {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return entities.Claw{}, sql.ErrNotFound
	}

	existing.Status = entities.StatusStop
	existing.UpdatedAt = time.Now()
	s.claws[existing.ID] = existing

	return existing, nil
}

func (s *fakeClawService) Delete(
	_ context.Context,
	cm clawcommands.DeleteClaw,
) error {
	existing, ok := s.claws[cm.ClawID]
	if !ok || existing.UserID != cm.UserID {
		return sql.ErrNotFound
	}

	delete(s.claws, cm.ClawID)

	return nil
}
