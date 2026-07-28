package hostingapi

import (
	"encoding/json"
	"testing"
)

func TestLifecycleCommandRequestJSON(t *testing.T) {
	t.Parallel()

	req := LifecycleCommandRequest{
		OperationID:    "op-1",
		IdempotencyKey: "claw-1:start",
		UserID:         "user-1",
		ClawID:         "claw-1",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"operationId":"op-1","idempotencyKey":"claw-1:start","userId":"user-1","clawId":"claw-1"}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}

func TestEnsureRuntimeRequestJSON(t *testing.T) {
	t.Parallel()

	req := EnsureRuntimeRequest{
		UserID: "user-1",
		ClawID: "claw-1",
		Vars:   []string{"OPENROUTER_API_KEY=secret"},
		ClawConfig: []ClawConfigFile{
			{Name: "openclaw", FileType: "json", Data: "{}"},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"userId":"user-1","clawId":"claw-1","vars":["OPENROUTER_API_KEY=secret"],"clawConfig":[{"name":"openclaw","fileType":"json","data":"{}"}]}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}

func TestEnsureRuntimeResponseJSON(t *testing.T) {
	t.Parallel()

	resp := EnsureRuntimeResponse{
		RuntimeRecordID:   "runtime-1",
		DockerContainerID: "docker-1",
		Port:              BoundTCPPort("8080"),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"runtimeRecordId":"runtime-1","dockerContainerId":"docker-1","port":"8080"}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}

func TestRuntimeStateResponseJSON(t *testing.T) {
	t.Parallel()

	resp := RuntimeStateResponse{
		RuntimeRecordID:   "runtime-1",
		DockerContainerID: "docker-1",
		ObservedState:     RuntimeObservedState("running"),
		RuntimeStatus:     RuntimeExecutionStatus("healthy"),
		Port:              BoundTCPPort("8080"),
		LastError:         "none",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"runtimeRecordId":"runtime-1","dockerContainerId":"docker-1","observedState":"running","runtimeStatus":"healthy","port":"8080","lastError":"none"}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}

func TestRuntimeStateResponseJSONOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	resp := RuntimeStateResponse{
		ObservedState: RuntimeObservedState("stopped"),
		RuntimeStatus: RuntimeExecutionStatus("idle"),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"observedState":"stopped","runtimeStatus":"idle"}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}

func TestLifecycleRoutesAndErrorCodes(t *testing.T) {
	t.Parallel()

	if ClawsStartEndpoint != "/claws/start" {
		t.Fatalf("start endpoint = %q, want %q", ClawsStartEndpoint, "/claws/start")
	}
	if ClawsEnsureEndpoint != "/claws/ensure" {
		t.Fatalf("ensure endpoint = %q, want %q", ClawsEnsureEndpoint, "/claws/ensure")
	}
	if ClawsStopEndpoint != "/claws/stop" {
		t.Fatalf("stop endpoint = %q, want %q", ClawsStopEndpoint, "/claws/stop")
	}
	if ClawsDeleteEndpoint != "/claws/delete" {
		t.Fatalf("delete endpoint = %q, want %q", ClawsDeleteEndpoint, "/claws/delete")
	}
	if ClawsStateEndpoint != "/claws/state" {
		t.Fatalf("state endpoint = %q, want %q", ClawsStateEndpoint, "/claws/state")
	}

	if ErrCodeConflictInProgress != "lifecycle_in_progress" {
		t.Fatalf("conflict code = %q, want %q", ErrCodeConflictInProgress, "lifecycle_in_progress")
	}
	if ErrCodeUnknownState != "runtime_state_unknown" {
		t.Fatalf("unknown state code = %q, want %q", ErrCodeUnknownState, "runtime_state_unknown")
	}
}
