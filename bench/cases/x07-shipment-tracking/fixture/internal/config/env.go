package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env is the process environment a configuration is read from.
type Env map[string]string

// FromEnviron copies the process environment. Only the keys the service reads are kept, so a
// configuration cannot be handed a value it did not ask for and a status page cannot print one.
func FromEnviron() Env {
	env := Env{}
	for _, key := range []string{
		KeyRoot, KeyAddr, KeyTenants, KeySecret, KeyTokenTTL, KeyZone, KeyPageSize, KeyMaxBody,
		KeyMaxBatch, KeyMaxGrams, KeyMaxNote, KeyMaxLines, KeyMaxResp, KeyMaxEvents, KeyMaxShips,
		KeyCloseWin, KeyMaxCells, KeyTrailCap, KeyTaxRate, KeyEgress, KeyInsecure, KeyRenderer,
	} {
		if value, ok := os.LookupEnv(key); ok {
			env[key] = value
		}
	}
	return env
}

// Get returns a value, or "" when the key is unset or empty.
func (e Env) Get(key string) string { return e[key] }

// Set records a value. It is how a test writes the deployment it means.
func (e Env) Set(key, value string) { e[key] = value }

// Required returns a value that must be present. A value that is only whitespace counts as absent: a
// secret of spaces is not a secret.
func (e Env) Required(key string) (string, error) {
	value, ok := e[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("config: %s is required", key)
	}
	return value, nil
}

// CSV reads a comma-separated list. Entries are trimmed and an empty entry is refused in a list that
// carries more than one: "a,,b" is a mistake, not a list of two.
func (e Env) CSV(key string) ([]string, error) {
	raw, ok := e[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	list := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry == "" {
			return nil, fmt.Errorf("config: %s carries an empty entry", key)
		}
		if seen[entry] {
			return nil, fmt.Errorf("config: %s repeats %q", key, entry)
		}
		seen[entry] = true
		list = append(list, entry)
	}
	return list, nil
}

// Int reads an integer. A value with leading or trailing space is refused: a number is written as a
// number, not as text a reader has to trim.
func (e Env) Int(key string, fallback int) (int, error) {
	raw, ok := e[key]
	if !ok || raw == "" {
		return fallback, nil
	}
	if strings.TrimSpace(raw) != raw {
		return 0, fmt.Errorf("config: %s carries surrounding space", key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not an integer", key, raw)
	}
	return value, nil
}

// Int64 reads a 64-bit integer.
func (e Env) Int64(key string, fallback int64) (int64, error) {
	raw, ok := e[key]
	if !ok || raw == "" {
		return fallback, nil
	}
	if strings.TrimSpace(raw) != raw {
		return 0, fmt.Errorf("config: %s carries surrounding space", key)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not an integer", key, raw)
	}
	return value, nil
}

// Int32 reads a 32-bit integer, which is the type a basis point rate is carried in.
func (e Env) Int32(key string, fallback int32) (int32, error) {
	value, err := e.Int64(key, int64(fallback))
	if err != nil {
		return 0, err
	}
	if value < 0 || value > 1<<31-1 {
		return 0, fmt.Errorf("config: %s=%d is out of range", key, value)
	}
	return int32(value), nil
}

// Duration reads a Go duration. A bare number is refused: "60" is not a duration, and reading it as
// one would make a deployment's token lifetime depend on a convention.
func (e Env) Duration(key string, fallback time.Duration) (time.Duration, error) {
	raw, ok := e[key]
	if !ok || raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not a duration", key, raw)
	}
	return parsed, nil
}

// Bool reads a boolean in the exact form the service writes: "true" or "false".
func (e Env) Bool(key string, fallback bool) (bool, error) {
	raw, ok := e[key]
	if !ok || raw == "" {
		return fallback, nil
	}
	switch raw {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("config: %s=%q is not true or false", key, raw)
	}
}

// requiredPath reads the root. It must be present and absolute; the rest of the checks are in
// CheckRoot, which also looks at the filesystem.
func requiredPath(env Env, key string) (string, error) {
	value, err := env.Required(key)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("config: %s=%q is not absolute", key, value)
	}
	return value, nil
}
