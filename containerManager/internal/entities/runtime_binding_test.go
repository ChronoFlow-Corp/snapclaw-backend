package entities

import (
	"encoding/json"
	"testing"
)

func TestRuntimeBindingManifest_MarshalJSON(t *testing.T) {
	manifest := RuntimeBindingManifest{
		Bindings: []RuntimeBinding{
			{
				Name:                  "gmail-pubsub",
				Kind:                  "http-sidecar",
				Provider:              "gmail",
				InternalPath:          "/gmail-pubsub",
				RequiresSecondaryPort: true,
			},
		},
	}

	gotJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	const want = `{"bindings":[{"name":"gmail-pubsub","kind":"http-sidecar","provider":"gmail","internalPath":"/gmail-pubsub","requiresSecondaryPort":true}]}`
	if string(gotJSON) != want {
		t.Fatalf("manifest json = %s, want %s", string(gotJSON), want)
	}
}
