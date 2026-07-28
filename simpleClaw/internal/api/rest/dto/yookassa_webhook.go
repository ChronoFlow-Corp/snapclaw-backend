package dto

import "encoding/json"

type YooKassaWebhookRequest struct {
	Type   string          `json:"type"`
	Event  string          `json:"event"`
	Object json.RawMessage `json:"object"`
}
