package config

import (
	"testing"
	"time"
)

const secret = "0123456789abcdef0123456789abcdef"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaults(t *testing.T) {
	c, err := load(env(map[string]string{"DATABASE_URL": "postgres://x", "JWT_SECRET": secret}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPPort != "8080" || c.LogLevel != "info" || c.DBMaxConns != 10 || c.DBMinConns != 1 ||
		c.DBMaxConnLifetime != 30*time.Minute || c.DBHealthCheck != time.Minute ||
		c.OllamaURL != "http://localhost:11434" || c.EmbeddingModel != "nomic-embed-text" || c.EmbeddingTimeout != 30*time.Second {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestOverrides(t *testing.T) {
	c, err := load(env(map[string]string{
		"DATABASE_URL": "postgres://x", "JWT_SECRET": secret, "HTTP_PORT": "9000", "DB_MAX_CONNS": "20",
		"DB_MAX_CONN_LIFETIME": "5m", "OLLAMA_URL": "http://ollama:11434", "EMBEDDING_MODEL": "m", "EMBEDDING_TIMEOUT": "5s",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPPort != "9000" || c.DBMaxConns != 20 || c.DBMaxConnLifetime != 5*time.Minute ||
		c.OllamaURL != "http://ollama:11434" || c.EmbeddingModel != "m" || c.EmbeddingTimeout != 5*time.Second {
		t.Fatalf("overrides not applied: %+v", c)
	}
}

func TestInvalid(t *testing.T) {
	cases := map[string]map[string]string{
		"missing url":    {"JWT_SECRET": secret},
		"missing secret": {"DATABASE_URL": "x"},
		"short secret":   {"DATABASE_URL": "x", "JWT_SECRET": "short"},
		"bad int":        {"DATABASE_URL": "x", "JWT_SECRET": secret, "DB_MAX_CONNS": "abc"},
		"min over max":   {"DATABASE_URL": "x", "JWT_SECRET": secret, "DB_MAX_CONNS": "2", "DB_MIN_CONNS": "5"},
		"bad duration":   {"DATABASE_URL": "x", "JWT_SECRET": secret, "DB_MAX_CONN_LIFETIME": "soon"},
	}
	for name, m := range cases {
		if _, err := load(env(m)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
