package notifications

import (
	"context"
	"errors"
	"testing"
)

type mockGCPClient struct {
	secrets map[string][]byte
	err     error
}

func (m *mockGCPClient) AccessSecretVersion(_ context.Context, name string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	val, ok := m.secrets[name]
	if !ok {
		return nil, errors.New("NOT_FOUND")
	}
	return val, nil
}

func TestGCPProviderMatch(t *testing.T) {
	p := NewGCPSecretManagerProvider(nil)
	if !p.Match("gcp-sm://projects/myproj/secrets/token") {
		t.Fatal("expected match for gcp-sm:// prefix")
	}
	if p.Match("aws-sm://secret") {
		t.Fatal("unexpected match for aws-sm:// prefix")
	}
	if p.Match("env://KEY") {
		t.Fatal("unexpected match for env:// prefix")
	}
}

func TestGCPProviderResolve(t *testing.T) {
	client := &mockGCPClient{secrets: map[string][]byte{
		"projects/myproj/secrets/slack-token/versions/latest": []byte("xoxb-gcp-token"),
	}}
	p := NewGCPSecretManagerProvider(client)

	val, err := p.Resolve(context.Background(), "gcp-sm://projects/myproj/secrets/slack-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "xoxb-gcp-token" {
		t.Fatalf("expected xoxb-gcp-token, got %q", val)
	}
}

func TestGCPProviderResolveWithVersion(t *testing.T) {
	client := &mockGCPClient{secrets: map[string][]byte{
		"projects/myproj/secrets/token/versions/3": []byte("versioned-value"),
	}}
	p := NewGCPSecretManagerProvider(client)

	val, err := p.Resolve(context.Background(), "gcp-sm://projects/myproj/secrets/token/versions/3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "versioned-value" {
		t.Fatalf("expected versioned-value, got %q", val)
	}
}

func TestGCPProviderResolveNotFound(t *testing.T) {
	client := &mockGCPClient{secrets: map[string][]byte{}}
	p := NewGCPSecretManagerProvider(client)

	_, err := p.Resolve(context.Background(), "gcp-sm://projects/myproj/secrets/missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestGCPProviderResolveEmptyKey(t *testing.T) {
	p := NewGCPSecretManagerProvider(nil)

	_, err := p.Resolve(context.Background(), "gcp-sm://")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestGCPProviderResolveClientError(t *testing.T) {
	client := &mockGCPClient{err: errors.New("permission denied")}
	p := NewGCPSecretManagerProvider(client)

	_, err := p.Resolve(context.Background(), "gcp-sm://projects/myproj/secrets/token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}
