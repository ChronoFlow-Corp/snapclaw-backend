package entities

// OpenRouterKey keeps both the identifier issued by OpenRouter
// and the secret token that should be used to access their API.
type OpenRouterKey struct {
	ID     string
	Secret string
}
