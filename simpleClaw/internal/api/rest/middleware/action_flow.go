package middleware

import (
	"strings"
)

func ClassifyActionFlow(method, path string) (string, string) {
	return classifySimpleClawActionFlow(method, path)
}

func classifySimpleClawActionFlow(method, path string) (string, string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeActionPath(path)

	switch path {
	case "/health":
		return "health.check", "health"
	case "/pubsub":
		return "pubsub.forward", "gmail_pubsub_fanout"
	case "/api/billing/webhook/yookassa":
		if method == "POST" {
			return "payment.webhook.receive", "billing_payment"
		}
	case "/api/billing/webhook/openrouter":
		if method == "POST" {
			return "usage.webhook.receive", "billing_usage"
		}
	}

	if strings.HasPrefix(path, "/api/auth/") {
		switch {
		case strings.HasSuffix(path, "/callback"):
			return "auth.callback", "auth"
		case method == "POST" && strings.HasSuffix(path, "/refresh"):
			return "auth.refresh", "auth"
		default:
			return "auth.connect", "auth"
		}
	}

	if strings.HasPrefix(path, "/api/me/connect/") {
		switch {
		case strings.HasSuffix(path, "/callback"):
			return "channel.connect.callback", "channel_connect"
		default:
			return "channel.connect", "channel_connect"
		}
	}

	if strings.HasPrefix(path, "/api/me") || strings.HasPrefix(path, "/api/user-info") || strings.HasPrefix(path, "/api/channel") {
		switch {
		case method == "GET" && strings.Contains(path, "user-info"):
			return "user.info.get", "user_profile"
		case method == "POST" && strings.Contains(path, "channel"):
			return "channel.add", "channel_connect"
		default:
			return "user.request", "user_profile"
		}
	}

	if path == "/api/claws" {
		switch method {
		case "GET":
			return "claw.list", "claw_lifecycle"
		case "POST":
			return "claw.create", "claw_lifecycle"
		}
	}

	if strings.HasPrefix(path, "/api/claws/") {
		switch {
		case method == "GET":
			return "claw.get", "claw_lifecycle"
		case method == "PUT":
			return "claw.update", "claw_lifecycle"
		case method == "DELETE":
			return "claw.delete", "claw_lifecycle"
		case method == "POST" && strings.HasSuffix(path, "/start"):
			return "claw.start", "claw_lifecycle"
		case method == "POST" && strings.HasSuffix(path, "/stop"):
			return "claw.stop", "claw_lifecycle"
		case method == "POST" && strings.HasSuffix(path, "/approve"):
			return "claw.approve", "claw_pairing"
		case method == "POST" && strings.HasSuffix(path, "/connect"):
			return "claw.connect", "claw_pairing"
		}
	}

	if path == "/api/servers" {
		switch method {
		case "GET":
			return "server.list", "server_registry"
		case "POST":
			return "server.create", "server_registry"
		}
	}

	if strings.HasPrefix(path, "/api/servers/") {
		switch method {
		case "GET":
			return "server.get", "server_registry"
		case "PUT":
			return "server.update", "server_registry"
		case "DELETE":
			return "server.delete", "server_registry"
		}
	}

	return "http.request", "http_request"
}

func normalizeActionPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}

	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}

	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "/"
	}

	return path
}
