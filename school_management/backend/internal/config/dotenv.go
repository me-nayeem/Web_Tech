package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// readEnvFile reads local development settings. The caller gives process
// environment variables precedence, and the file is never logged.
func readEnvFile(path string) (map[string]string, error) {
	values := map[string]string{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return values, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for line := 1; scanner.Scan(); line++ {
		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		key, raw, ok := strings.Cut(s, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvKey(key) {
			return nil, fmt.Errorf("invalid %s line %d: expected KEY=value", path, line)
		}
		v, err := parseEnvValue(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid %s line %d for %s: %w", path, line, key, err)
		}
		values[key] = v
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	return values, nil
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, c := range key {
		if c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func parseEnvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	if raw[0] == '"' || raw[0] == '\'' {
		quote := raw[0]
		var out strings.Builder
		for i := 1; i < len(raw); i++ {
			if raw[i] == quote {
				tail := strings.TrimSpace(raw[i+1:])
				if tail != "" && !strings.HasPrefix(tail, "#") {
					return "", errors.New("unexpected text after closing quote")
				}
				return out.String(), nil
			}
			if quote == '"' && raw[i] == '\\' && i+1 < len(raw) {
				switch raw[i+1] {
				case '"', '\\':
					out.WriteByte(raw[i+1])
					i++
					continue
				case 'n':
					out.WriteByte('\n')
					i++
					continue
				case 'r':
					out.WriteByte('\r')
					i++
					continue
				}
			}
			out.WriteByte(raw[i])
		}
		return "", errors.New("missing closing quote")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && (i == 0 || raw[i-1] == ' ' || raw[i-1] == '\t') {
			raw = raw[:i]
			break
		}
	}
	return strings.TrimSpace(raw), nil
}
