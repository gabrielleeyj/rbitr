package notifications

import (
	"context"
	"errors"
	"testing"
)

type mockAWSClient struct {
	secrets map[string]string
	err     error
}

func (m *mockAWSClient) GetSecretValue(_ context.Context, secretID string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	val, ok := m.secrets[secretID]
	if !ok {
		return "", errors.New("ResourceNotFoundException")
	}
	return val, nil
}

func TestAWSProviderMatch(t *testing.T) {
	p := NewAWSSecretsManagerProvider(nil)
	if !p.Match("aws-sm://my-secret") {
		t.Fatal("expected match for aws-sm:// prefix")
	}
	if p.Match("env://KEY") {
		t.Fatal("unexpected match for env:// prefix")
	}
	if p.Match("gcp-sm://projects/p/secrets/s") {
		t.Fatal("unexpected match for gcp-sm:// prefix")
	}
}

func TestAWSProviderResolve(t *testing.T) {
	client := &mockAWSClient{secrets: map[string]string{
		"rbitr/slack-token": "xoxb-test-token",
	}}
	p := NewAWSSecretsManagerProvider(client)

	val, err := p.Resolve(context.Background(), "aws-sm://rbitr/slack-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "xoxb-test-token" {
		t.Fatalf("expected xoxb-test-token, got %q", val)
	}
}

func TestAWSProviderResolveNotFound(t *testing.T) {
	client := &mockAWSClient{secrets: map[string]string{}}
	p := NewAWSSecretsManagerProvider(client)

	_, err := p.Resolve(context.Background(), "aws-sm://missing-secret")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAWSProviderResolveEmptyKey(t *testing.T) {
	client := &mockAWSClient{secrets: map[string]string{}}
	p := NewAWSSecretsManagerProvider(client)

	_, err := p.Resolve(context.Background(), "aws-sm://")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAWSProviderResolveClientError(t *testing.T) {
	client := &mockAWSClient{err: errors.New("access denied")}
	p := NewAWSSecretsManagerProvider(client)

	_, err := p.Resolve(context.Background(), "aws-sm://rbitr/token")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestAWSProviderResolveRedactsRef(t *testing.T) {
	client := &mockAWSClient{secrets: map[string]string{}}
	p := NewAWSSecretsManagerProvider(client)

	_, err := p.Resolve(context.Background(), "aws-sm://very-long-secret-name-that-should-be-redacted")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrSecretNotFound) {
		errMsg := err.Error()
		if len(errMsg) > 80 {
			t.Fatalf("error message too long, may leak secret path: %q", errMsg)
		}
	}
}
