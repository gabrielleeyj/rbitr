package notifications

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVaultProviderMatch(t *testing.T) {
	p := NewVaultProvider("https://vault.example.com", "token")
	if !p.Match("vault://secret/data/myapp/creds") {
		t.Fatal("expected match for vault:// prefix")
	}
	if p.Match("aws-sm://secret") {
		t.Fatal("unexpected match for aws-sm:// prefix")
	}
	if p.Match("env://KEY") {
		t.Fatal("unexpected match for env:// prefix")
	}
}

func TestVaultProviderResolve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path != "/v1/secret/data/rbitr/slack" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"token":"vault-secret-value"}}}`))
	}))
	defer srv.Close()

	p := NewVaultProvider(srv.URL, "test-token")
	val, err := p.Resolve(context.Background(), "vault://secret/data/rbitr/slack#token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "vault-secret-value" {
		t.Fatalf("expected vault-secret-value, got %q", val)
	}
}

func TestVaultProviderResolveSingleKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"value":"only-one"}}}`))
	}))
	defer srv.Close()

	p := NewVaultProvider(srv.URL, "tok")
	val, err := p.Resolve(context.Background(), "vault://secret/data/app/single")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "only-one" {
		t.Fatalf("expected only-one, got %q", val)
	}
}

func TestVaultProviderResolveMultiKeyNoSelector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"data":{"user":"admin","pass":"secret"}}}`))
	}))
	defer srv.Close()

	p := NewVaultProvider(srv.URL, "tok")
	_, err := p.Resolve(context.Background(), "vault://secret/data/app/multi")
	if err == nil {
		t.Fatal("expected error for multi-key without selector")
	}
}

func TestVaultProviderResolveNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewVaultProvider(srv.URL, "tok")
	_, err := p.Resolve(context.Background(), "vault://secret/data/missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestVaultProviderResolveEmptyPath(t *testing.T) {
	p := NewVaultProvider("https://vault.example.com", "tok")
	_, err := p.Resolve(context.Background(), "vault://")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestVaultProviderResolveRedactsRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewVaultProvider(srv.URL, "tok")
	_, err := p.Resolve(context.Background(), "vault://secret/data/very-long-path-that-should-be-redacted")
	if err == nil {
		t.Fatal("expected error")
	}
	errMsg := err.Error()
	if len(errMsg) > 80 {
		t.Fatalf("error message too long, may leak secret path: %q", errMsg)
	}
}
