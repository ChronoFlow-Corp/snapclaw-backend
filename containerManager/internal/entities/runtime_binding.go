package entities

type RuntimeBindingManifest struct {
	Bindings []RuntimeBinding `json:"bindings,omitempty"`
}

type RuntimeBinding struct {
	Name                  string `json:"name,omitempty"`
	Kind                  string `json:"kind,omitempty"`
	Provider              string `json:"provider,omitempty"`
	InternalPath          string `json:"internalPath,omitempty"`
	RequiresSecondaryPort bool   `json:"requiresSecondaryPort,omitempty"`
}
