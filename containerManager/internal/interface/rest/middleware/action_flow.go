package middleware

import "strings"

func ClassifyActionFlow(method, path string) (string, string) {
	return classifyContainerManagerActionFlow(method, path)
}

func classifyContainerManagerActionFlow(method, path string) (string, string) {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = normalizeActionPath(path)

	switch path {
	case "/claws":
		switch method {
		case "POST":
			return "claw.create", "claw_lifecycle"
		case "PUT":
			return "claw.update", "claw_lifecycle"
		case "DELETE":
			return "claw.delete", "claw_lifecycle"
		}
	case "/claws/start":
		return "claw.start", "claw_lifecycle"
	case "/claws/stop":
		return "claw.stop", "claw_lifecycle"
	case "/claws/config":
		if method == "GET" {
			return "claw.config.archive", "claw_lifecycle"
		}

		if method == "POST" {
			return "claw.config.restore", "claw_lifecycle"
		}
	case "/approve":
		return "claw.approve", "claw_pairing"
	case "/connect":
		return "claw.connect", "claw_pairing"
	case "/gmail-pubsub":
		return "pubsub.forward", "gmail_pubsub_fanout"
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
