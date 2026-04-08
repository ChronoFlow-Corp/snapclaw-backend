package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"containermanager/internal/entities"
	"containermanager/internal/infrastucture/sql/storage"
	"containermanager/internal/service/commands"
	"shared/pkg/hostingapi"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func TestClawRegisterUsesAsyncLifecycleRoutes(t *testing.T) {
	t.Parallel()

	c := NewClaw(&fakeClawService{}, "secret", ClawOptions{})
	r := chi.NewRouter()
	c.Register(r)

	var routes []string
	if err := chi.Walk(r, func(method string, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes = append(routes, method+" "+route)
		return nil
	}); err != nil {
		t.Fatalf("walk routes: %v", err)
	}

	sort.Strings(routes)

	want := []string{
		"GET /approve",
		"GET /capacity",
		"GET /claws/config",
		"GET /claws/state",
		"POST /connect",
		"POST /claws/delete",
		"POST /claws/ensure",
		"POST /gmail-pubsub",
		"POST /claws/start",
		"POST /claws/stop",
		"POST /claws/config",
	}

	// Old create/update routes must not remain in the lifecycle API.
	for _, forbidden := range []string{"POST /claws", "PUT /claws"} {
		for _, route := range routes {
			if route == forbidden {
				t.Fatalf("found forbidden route %q in %v", forbidden, routes)
			}
		}
	}

	for _, expected := range want {
		if !containsString(routes, expected) {
			t.Fatalf("missing route %q in %v", expected, routes)
		}
	}
}

func TestClawStartAcceptsLifecycleCommandRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeClawService{}
	c := NewClaw(fake, "secret", ClawOptions{})

	reqBody := hostingapi.LifecycleCommandRequest{
		OperationID:    "op-1",
		IdempotencyKey: "claw-1:start",
		UserID:         "user-1",
		ClawID:         "claw-1",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/claws/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c.Start(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	if !fake.startCalled {
		t.Fatal("expected service start to be called")
	}

	if fake.startCmd.UserID != reqBody.UserID || fake.startCmd.ClawID != reqBody.ClawID {
		t.Fatalf("start command = %+v, want user=%s claw=%s", fake.startCmd, reqBody.UserID, reqBody.ClawID)
	}
}

func TestClawEnsureAcceptsEnsureRuntimeRequest(t *testing.T) {
	t.Parallel()

	fake := &fakeClawService{
		ensureResult: entities.Container{
			ID:          uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			ContainerID: "docker-1",
			Port:        "8080",
		},
	}
	c := NewClaw(fake, "secret", ClawOptions{})

	reqBody := hostingapi.EnsureRuntimeRequest{
		UserID: "user-1",
		ClawID: "claw-1",
		Vars:   []string{"OPENROUTER_API_KEY=secret"},
		ClawConfig: []hostingapi.ClawConfigFile{
			{Name: "openclaw", FileType: "json", Data: "{}"},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/claws/ensure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	c.Ensure(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if !fake.ensureCalled {
		t.Fatal("expected service ensure to be called")
	}

	if fake.ensureCmd.UserID != reqBody.UserID || fake.ensureCmd.ClawID != reqBody.ClawID {
		t.Fatalf("ensure command = %+v, want user=%s claw=%s", fake.ensureCmd, reqBody.UserID, reqBody.ClawID)
	}

	var resp hostingapi.EnsureRuntimeResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.RuntimeRecordID != fake.ensureResult.ID.String() {
		t.Fatalf("runtime record id = %q, want %q", resp.RuntimeRecordID, fake.ensureResult.ID.String())
	}
}

func TestClawStateReturnsRuntimeStateResponse(t *testing.T) {
	t.Parallel()

	containerID := uuid.New()
	clawID := uuid.New()
	userID := "user-1"
	fake := &fakeClawService{
		runtimeState: entities.Container{
			ID:             containerID,
			UserID:         userID,
			ClawID:         clawID.String(),
			ContainerID:    "docker-1",
			Status:         entities.ContainerStatusRunning,
			Port:           "8080",
			HasStartedOnce: true,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		},
	}

	c := NewClaw(fake, "secret", ClawOptions{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/claws/state?userId="+userID+"&clawId="+clawID.String(), http.NoBody)

	c.State(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got hostingapi.RuntimeStateResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.RuntimeRecordID != containerID.String() {
		t.Fatalf("runtime record id = %q, want %q", got.RuntimeRecordID, containerID.String())
	}

	if got.DockerContainerID != "docker-1" {
		t.Fatalf("docker container id = %q, want %q", got.DockerContainerID, "docker-1")
	}

	if got.ObservedState != hostingapi.RuntimeObservedState(entities.ContainerStatusRunning) {
		t.Fatalf("observed state = %q, want %q", got.ObservedState, entities.ContainerStatusRunning)
	}

	if got.RuntimeStatus != hostingapi.RuntimeExecutionStatus(entities.ContainerStatusRunning) {
		t.Fatalf("runtime status = %q, want %q", got.RuntimeStatus, entities.ContainerStatusRunning)
	}

	if got.Port != hostingapi.BoundTCPPort("8080") {
		t.Fatalf("port = %q, want %q", got.Port, "8080")
	}
}

func TestClawStateMapsMissingRuntimeToUnknownState(t *testing.T) {
	t.Parallel()

	fake := &fakeClawService{runtimeStateErr: storage.ErrNotFound}
	c := NewClaw(fake, "secret", ClawOptions{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/claws/state?userId=user-1&clawId="+uuid.NewString(), http.NoBody)

	c.State(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var payload hostingapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}

	if payload.Code != hostingapi.ErrCodeUnknownState {
		t.Fatalf("code = %q, want %q", payload.Code, hostingapi.ErrCodeUnknownState)
	}
}

type fakeClawService struct {
	ensureCalled    bool
	ensureCmd       commands.CreateClaw
	ensureResult    entities.Container
	startCalled     bool
	startCmd        commands.StartClaw
	approveCalled   bool
	approveUserID   string
	approveClawID   string
	approveChannel  string
	approveCode     string
	runtimeState    entities.Container
	runtimeStateErr error
}

func (f *fakeClawService) Ensure(_ context.Context, cm commands.CreateClaw) (entities.Container, error) {
	f.ensureCalled = true
	f.ensureCmd = cm
	return f.ensureResult, nil
}

func (f *fakeClawService) Start(_ context.Context, cm commands.StartClaw) error {
	f.startCalled = true
	f.startCmd = cm
	return nil
}

func (f *fakeClawService) Approve(_ context.Context, clawID, userID, channelType, code string) error {
	f.approveCalled = true
	f.approveClawID = clawID
	f.approveUserID = userID
	f.approveChannel = channelType
	f.approveCode = code

	return nil
}

func (f *fakeClawService) Stop(context.Context, commands.StopClaw) error {
	panic("unexpected call")
}

func (f *fakeClawService) Update(context.Context, commands.UpdateClaw) error {
	panic("unexpected call")
}

func (f *fakeClawService) Delete(context.Context, commands.DeleteClaw) error {
	panic("unexpected call")
}

func (f *fakeClawService) Connect(context.Context, commands.ConnectCommand) error {
	panic("unexpected call")
}

func TestClawApprove_RequiresChannelType(t *testing.T) {
	t.Parallel()

	c := NewClaw(&fakeClawService{}, "secret", ClawOptions{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/approve?userId=user-1&clawId=claw-1&code=123456", http.NoBody)

	c.Approve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestClawApprove_PassesChannelTypeToService(t *testing.T) {
	t.Parallel()

	fake := &fakeClawService{}
	c := NewClaw(fake, "secret", ClawOptions{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/approve?userId=user-1&clawId=claw-1&channelType=telegram&code=123456", http.NoBody)

	c.Approve(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	if !fake.approveCalled {
		t.Fatal("expected approve to be called")
	}

	if fake.approveChannel != "telegram" {
		t.Fatalf("channel = %q, want %q", fake.approveChannel, "telegram")
	}
}

func (f *fakeClawService) ConfigArchive(context.Context, commands.ConfigArchive, io.Writer) error {
	panic("unexpected call")
}

func (f *fakeClawService) RestoreConfig(context.Context, commands.RestoreConfig, io.Reader) error {
	panic("unexpected call")
}

func (f *fakeClawService) ListRunning(context.Context) ([]entities.Container, error) {
	panic("unexpected call")
}

func (f *fakeClawService) RuntimeState(_ context.Context, _ commands.StateClaw) (entities.Container, error) {
	if f.runtimeStateErr != nil {
		return entities.Container{}, f.runtimeStateErr
	}

	return f.runtimeState, nil
}

func (f *fakeClawService) RuntimeBinding(
	context.Context,
	string,
	string,
	string,
) (entities.RuntimeBinding, error) {
	panic("unexpected call")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
