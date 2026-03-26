package dto

type OpenRouterWebhookRequest struct {
	ResourceSpans []OpenRouterResourceSpan `json:"resourceSpans"`
}

type OpenRouterResourceSpan struct {
	ScopeSpans []OpenRouterScopeSpan `json:"scopeSpans"`
}

type OpenRouterScopeSpan struct {
	Spans []OpenRouterSpan `json:"spans"`
}

type OpenRouterSpan struct {
	TraceID         string                    `json:"traceId"`
	SpanID          string                    `json:"spanId"`
	EndTimeUnixNano string                    `json:"endTimeUnixNano"`
	Attributes      []OpenRouterSpanAttribute `json:"attributes"`
}

type OpenRouterSpanAttribute struct {
	Key   string                   `json:"key"`
	Value OpenRouterAttributeValue `json:"value"`
}

type OpenRouterAttributeValue struct {
	StringValue string  `json:"stringValue"`
	IntValue    string  `json:"intValue"`
	DoubleValue float64 `json:"doubleValue"`
}
