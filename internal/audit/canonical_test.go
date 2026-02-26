package audit

import (
	"bytes"
	"testing"
)

func TestCanonicalJSONDeterministic(t *testing.T) {
	input := map[string]any{
		"b": 1,
		"a": map[string]any{"z": "last", "m": "mid"},
		"c": []any{"x", "y"},
	}
	first, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := CanonicalJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("expected deterministic canonical json")
	}
}
