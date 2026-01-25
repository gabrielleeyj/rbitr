package notifications

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompositeResolverEnv(t *testing.T) {
	t.Setenv("RBTR_TEST_SECRET", "value")

	resolver := NewCompositeResolver([]SecretProvider{EnvProvider{}}, time.Minute)
	value, err := resolver.Resolve(context.Background(), "env://RBTR_TEST_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "value" {
		t.Fatalf("expected value, got %q", value)
	}
}

func TestCompositeResolverFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(path, []byte("token\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	resolver := NewCompositeResolver([]SecretProvider{FileProvider{}}, time.Minute)
	value, err := resolver.Resolve(context.Background(), "file://"+path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != "token" {
		t.Fatalf("expected token, got %q", value)
	}
}

func TestCompositeResolverCache(t *testing.T) {
	t.Setenv("RBTR_TEST_SECRET_CACHE", "value1")

	resolver := NewCompositeResolver([]SecretProvider{EnvProvider{}}, time.Minute)
	first, err := resolver.Resolve(context.Background(), "env://RBTR_TEST_SECRET_CACHE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != "value1" {
		t.Fatalf("expected value1, got %q", first)
	}

	_ = os.Setenv("RBTR_TEST_SECRET_CACHE", "value2")
	second, err := resolver.Resolve(context.Background(), "env://RBTR_TEST_SECRET_CACHE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second != "value1" {
		t.Fatalf("expected cached value1, got %q", second)
	}
}

func TestCompositeResolverMissing(t *testing.T) {
	resolver := NewCompositeResolver([]SecretProvider{EnvProvider{}}, time.Minute)
	_, err := resolver.Resolve(context.Background(), "env://RBTR_MISSING")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound")
	}
	if strings.Contains(err.Error(), "RBTR_MISSING") {
		t.Fatalf("error should be redacted, got %q", err.Error())
	}
}

func TestProvidersMatch(t *testing.T) {
	if !(EnvProvider{}).Match("env://KEY") {
		t.Fatalf("expected env match")
	}
	if (EnvProvider{}).Match("file://path") {
		t.Fatalf("unexpected env match")
	}
	if !(FileProvider{}).Match("file://path") {
		t.Fatalf("expected file match")
	}
	if (FileProvider{}).Match("env://KEY") {
		t.Fatalf("unexpected file match")
	}
}
