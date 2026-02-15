package utils

import (
	"strings"
	"testing"
)

func TestHashString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantSame bool // whether two calls produce the same result
	}{
		{name: "empty string", input: "", wantLen: 64, wantSame: true},
		{name: "simple input", input: "hello", wantLen: 64, wantSame: true},
		{name: "tenant key", input: "tenant_demo_key", wantLen: 64, wantSame: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HashString(tt.input)
			if len(got) != tt.wantLen {
				t.Errorf("HashString(%q) len = %d, want %d", tt.input, len(got), tt.wantLen)
			}
			if tt.wantSame && got != HashString(tt.input) {
				t.Errorf("HashString(%q) not deterministic", tt.input)
			}
		})
	}
	// Different inputs must produce different hashes
	if HashString("a") == HashString("b") {
		t.Error("HashString produced collision for 'a' and 'b'")
	}
}

func TestSecureCompare(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "equal strings", a: "abc", b: "abc", want: true},
		{name: "equal hashes", a: HashString("test"), b: HashString("test"), want: true},
		{name: "different strings", a: "abc", b: "def", want: false},
		{name: "different hashes", a: HashString("a"), b: HashString("b"), want: false},
		{name: "empty strings", a: "", b: "", want: true},
		{name: "one empty", a: "abc", b: "", want: false},
		{name: "different lengths", a: "short", b: "a longer string", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SecureCompare(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("SecureCompare(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGenerateAPIKey(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "generates valid key"},
		{name: "generates unique keys"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawKey, keyHash, prefix, err := GenerateAPIKey()
			if err != nil {
				t.Fatalf("GenerateAPIKey() error = %v", err)
			}
			if !strings.HasPrefix(rawKey, KeyPrefix) {
				t.Errorf("raw key missing prefix, got %q", rawKey[:20])
			}
			if len(keyHash) != 64 {
				t.Errorf("key hash len = %d, want 64", len(keyHash))
			}
			if HashString(rawKey) != keyHash {
				t.Error("key hash doesn't match HashString(rawKey)")
			}
			if !strings.HasPrefix(prefix, KeyPrefix) {
				t.Errorf("prefix missing KeyPrefix, got %q", prefix)
			}
			if len(prefix) > 14 {
				t.Errorf("prefix too long: %d", len(prefix))
			}
		})
	}

	// Uniqueness check
	raw1, _, _, _ := GenerateAPIKey()
	raw2, _, _, _ := GenerateAPIKey()
	if raw1 == raw2 {
		t.Error("GenerateAPIKey produced duplicate keys")
	}
}
