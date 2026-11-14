// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// DatabaseURL connects as the restricted app role (never owner/superuser).
	DatabaseURL string
	HTTPPort    string
	LogLevel    string
	// JWTSecret signs and verifies dev auth tokens (HS256); at least 32 bytes.
	JWTSecret string

	// Embedding service (Ollama).
	OllamaURL        string
	EmbeddingModel   string
	EmbeddingTimeout time.Duration

	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration
	DBHealthCheck     time.Duration
}

// Load reads configuration from the process environment.
func Load() (Config, error) { return load(os.Getenv) }

func load(get func(string) string) (Config, error) {
	c := Config{
		DatabaseURL: get("DATABASE_URL"),
		HTTPPort:    str(get, "HTTP_PORT", "8080"),
		LogLevel:    str(get, "LOG_LEVEL", "info"),
		JWTSecret:   get("JWT_SECRET"),

		OllamaURL:      str(get, "OLLAMA_URL", "http://localhost:11434"),
		EmbeddingModel: str(get, "EMBEDDING_MODEL", "nomic-embed-text"),
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("config: DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		return Config{}, errors.New("config: JWT_SECRET is required and must be at least 32 bytes")
	}
	var err error
	if c.DBMaxConns, err = int32v(get, "DB_MAX_CONNS", 10); err != nil {
		return Config{}, err
	}
	if c.DBMinConns, err = int32v(get, "DB_MIN_CONNS", 1); err != nil {
		return Config{}, err
	}
	if c.DBMinConns > c.DBMaxConns {
		return Config{}, errors.New("config: DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}
	if c.DBMaxConnLifetime, err = dur(get, "DB_MAX_CONN_LIFETIME", 30*time.Minute); err != nil {
		return Config{}, err
	}
	if c.EmbeddingTimeout, err = dur(get, "EMBEDDING_TIMEOUT", 30*time.Second); err != nil {
		return Config{}, err
	}
	if c.DBHealthCheck, err = dur(get, "DB_HEALTH_CHECK_PERIOD", time.Minute); err != nil {
		return Config{}, err
	}
	return c, nil
}

func str(get func(string) string, key, def string) string {
	if v := get(key); v != "" {
		return v
	}
	return def
}

func int32v(get func(string) string, key string, def int32) (int32, error) {
	v := get(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("config: %s must be a positive integer, got %q", key, v)
	}
	return int32(n), nil
}

func dur(get func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := get(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config: %s must be a positive duration, got %q", key, v)
	}
	return d, nil
}
