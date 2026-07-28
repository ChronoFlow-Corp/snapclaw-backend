package service

import (
	"fmt"
	"strconv"
	"strings"
)

func GogWatchPortFromGateway(gatewayPort string) (string, error) {
	portValue := strings.TrimSpace(gatewayPort)
	if portValue == "" {
		return "", fmt.Errorf("gateway port is required")
	}

	portNumber, err := strconv.Atoi(portValue)
	if err != nil {
		return "", fmt.Errorf("invalid gateway port %q: %w", gatewayPort, err)
	}

	watchPort := portNumber + 1
	if watchPort <= 0 || watchPort > 65535 {
		return "", fmt.Errorf("watch port out of range: %d", watchPort)
	}

	return strconv.Itoa(watchPort), nil
}
