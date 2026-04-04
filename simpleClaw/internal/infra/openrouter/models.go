package openrouter

type model struct {
	contain        string
	openrouterName string
}

var models []model = []model{
	{
		contain:        "sonnet",
		openrouterName: "anthropic/claude-sonnet-4.6",
	},
	{
		contain:        "gpt",
		openrouterName: "openai/gpt-5.4",
	},
	{
		contain:        "gemini",
		openrouterName: "google/gemini-3-flash-preview",
	},
	{
		contain:        "qwen",
		openrouterName: "qwen/qwen3.5-flash-02-23",
	},
	{
		contain:        "kimi",
		openrouterName: "moonshotai/kimi-k2.5",
	},
	{
		contain:        "glm",
		openrouterName: "z-ai/glm-5-turbo",
	},
}
