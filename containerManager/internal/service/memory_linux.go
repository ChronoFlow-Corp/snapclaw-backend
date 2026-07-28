//go:build linux

package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func readAvailableMemoryBytes() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}

		fields := strings.Fields(strings.TrimPrefix(line, "MemAvailable:"))
		if len(fields) == 0 {
			return 0, errors.New("MemAvailable value is missing")
		}

		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemAvailable: %w", err)
		}

		return value * 1024, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan /proc/meminfo: %w", err)
	}

	return 0, errors.New("MemAvailable is not present in /proc/meminfo")
}
