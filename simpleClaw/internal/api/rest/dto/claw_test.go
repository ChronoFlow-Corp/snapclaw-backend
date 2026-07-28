package dto

import (
	"encoding/json"
	"testing"
)

func TestCreateClawResponseJSONShape(t *testing.T) {
	resp := CreateClawResponse{
		ID:                 "claw-1",
		Name:               "main",
		DesiredState:       "running",
		ObservedState:      "running",
		LifecycleStatus:    "start_pending",
		CurrentOperationID: "op-1",
		LastError:          "boom",
		Telegram: ClawTelegramResponse{
			Connected: true,
			Status:    "connected",
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	const want = `{"id":"claw-1","name":"main","desiredState":"running","observedState":"running","lifecycleStatus":"start_pending","currentOperationId":"op-1","lastError":"boom","telegram":{"connected":true,"status":"connected"}}`
	if string(data) != want {
		t.Fatalf("json = %s, want %s", data, want)
	}
}
