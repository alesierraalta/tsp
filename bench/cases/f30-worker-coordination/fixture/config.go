package workqueue

import (
	"errors"
	"strings"
	"time"
)

type Config struct {
	LeaseTTL time.Duration
	Prefix   string
}

func (c Config) Validate() error {
	if c.LeaseTTL <= 0 {
		return errors.New("lease ttl must be positive")
	}
	if strings.TrimSpace(c.Prefix) == "" {
		return errors.New("prefix must be nonempty")
	}
	return nil
}

func DefaultConfig() Config {
	return Config{LeaseTTL: time.Minute, Prefix: "jobs"}
}

func validKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, "\n\r")
}

func namespaced(prefix, key string) string {
	return prefix + ":" + key
}
