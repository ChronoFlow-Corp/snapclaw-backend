package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	steadyStateBytesPerClaw = 800 * 1024 * 1024
	defaultReserveBytes     = 2 * 1024 * 1024 * 1024
)

type MaxClaws struct {
	Auto  bool
	Value int
}

func (m *MaxClaws) UnmarshalText(text []byte) error {
	raw := strings.TrimSpace(string(text))
	if raw == "" {
		return errors.New("max_claws is required")
	}

	if strings.EqualFold(raw, "auto") {
		m.Auto = true
		m.Value = 0
		return nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("parse max_claws: %w", err)
	}
	if value <= 0 {
		return errors.New("max_claws must be greater than zero")
	}

	m.Auto = false
	m.Value = value
	return nil
}

func (m *MaxClaws) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case int:
		return m.UnmarshalText([]byte(strconv.Itoa(v)))
	case int64:
		return m.UnmarshalText([]byte(strconv.FormatInt(v, 10)))
	case string:
		return m.UnmarshalText([]byte(v))
	default:
		return fmt.Errorf("unsupported max_claws type %T", raw)
	}
}

func (m MaxClaws) Resolve(totalBytes uint64, reserveBytes uint64) (int, error) {
	if !m.Auto {
		if m.Value <= 0 {
			return 0, errors.New("max_claws must be greater than zero")
		}

		return m.Value, nil
	}

	if totalBytes <= reserveBytes {
		return 0, errors.New("max_claws auto resolve: total memory does not exceed reserve")
	}

	value := int((totalBytes - reserveBytes) / steadyStateBytesPerClaw)
	if value <= 0 {
		return 0, errors.New("max_claws auto resolve: resolved capacity must be greater than zero")
	}

	return value, nil
}

func (m MaxClaws) ResolveLinux() (int, error) {
	totalBytes, err := readLinuxMemInfoValue("/proc/meminfo", "MemTotal")
	if err != nil {
		return 0, err
	}

	return m.Resolve(totalBytes, defaultReserveBytes)
}

func readLinuxMemInfoValue(path string, key string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	prefix := key + ":"
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		fields := strings.Fields(strings.TrimPrefix(line, prefix))
		if len(fields) == 0 {
			return 0, fmt.Errorf("parse %s: missing value", key)
		}

		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse %s: %w", key, err)
		}

		return value * 1024, nil
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}

	return 0, fmt.Errorf("%s not found in %s", key, path)
}
