package config

import (
	"os"
	"strconv"
)

// Env wraps access to environment variables.
// It keeps parsing logic centralized and easy to test.
type Env struct{}

func (Env) String(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

func (Env) Int(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	return def
}

func (Env) Bool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	default:
		return def
	}
}
