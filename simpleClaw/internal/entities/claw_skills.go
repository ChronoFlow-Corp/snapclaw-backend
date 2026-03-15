package entities

type SkillsConfig struct {
	AllowBundled []string              `json:"allowBundled,omitempty"`
	Load         SkillsLoad            `json:"load,omitempty"`
	Install      SkillsInstall         `json:"install,omitempty"`
	Entries      map[string]SkillEntry `json:"entries,omitempty"`
}

type SkillsLoad struct {
	ExtraDirs []string `json:"extraDirs,omitempty"`
}

type SkillsInstall struct {
	PreferBrew  bool   `json:"preferBrew,omitempty"`
	NodeManager string `json:"nodeManager,omitempty"`
}

type SkillEntry struct {
	Enabled bool              `json:"enabled,omitempty"`
	APIKey  string            `json:"apiKey,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
