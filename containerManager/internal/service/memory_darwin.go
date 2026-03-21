//go:build darwin

package service

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func readAvailableMemoryBytes() (uint64, error) {
	output, err := exec.Command("/usr/bin/vm_stat").Output()
	if err != nil {
		return 0, fmt.Errorf("run vm_stat: %w", err)
	}

	pageSize, err := parseDarwinPageSize(output)
	if err != nil {
		return 0, err
	}

	pageCounts, err := parseDarwinVMStatPages(output)
	if err != nil {
		return 0, err
	}

	availablePages := pageCounts["Pages free"] + pageCounts["Pages inactive"] + pageCounts["Pages speculative"]

	return availablePages * pageSize, nil
}

func parseDarwinPageSize(output []byte) (uint64, error) {
	firstLine, _, _ := bytes.Cut(output, []byte{'\n'})

	start := bytes.IndexByte(firstLine, '(')
	end := bytes.IndexByte(firstLine, ')')
	if start == -1 || end == -1 || end <= start {
		return 0, errors.New("vm_stat page size header is missing")
	}

	header := string(firstLine[start+1 : end])
	const prefix = "page size of "
	const suffix = " bytes"

	if !strings.HasPrefix(header, prefix) || !strings.HasSuffix(header, suffix) {
		return 0, fmt.Errorf("unexpected vm_stat header: %q", header)
	}

	value := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(header, prefix), suffix))

	pageSize, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse vm_stat page size: %w", err)
	}

	return pageSize, nil
}

func parseDarwinVMStatPages(output []byte) (map[string]uint64, error) {
	lines := strings.Split(string(output), "\n")
	counts := make(map[string]uint64, len(lines))

	for _, rawLine := range lines[1:] {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		name, valuePart, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		valueText := strings.TrimSpace(strings.TrimSuffix(valuePart, "."))
		value, err := strconv.ParseUint(valueText, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse vm_stat value for %q: %w", strings.TrimSpace(name), err)
		}

		counts[strings.TrimSpace(name)] = value
	}

	for _, key := range []string{"Pages free", "Pages inactive", "Pages speculative"} {
		if _, ok := counts[key]; !ok {
			return nil, fmt.Errorf("vm_stat value %q is missing", key)
		}
	}

	return counts, nil
}
