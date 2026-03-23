package license

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatcher_DetectsFileChange(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()
	assert.False(t, v.Info().Valid)

	// Create a valid key file.
	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))
	require.NoError(t, os.WriteFile(keyPath, token, 0600))

	w := NewWatcher(v, keyPath)
	w.snapshot()

	// Simulate a file change by modifying.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, os.WriteFile(keyPath, token, 0600))

	w.check()
	assert.True(t, v.Info().Valid)
	assert.Equal(t, "paid", v.Info().Tier)
}

func TestWatcher_DetectsFileRemoval(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")

	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))
	require.NoError(t, os.WriteFile(keyPath, token, 0600))

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()
	assert.True(t, v.Info().Valid)

	w := NewWatcher(v, keyPath)
	w.snapshot()

	// Remove the file.
	require.NoError(t, os.Remove(keyPath))
	w.check()

	assert.False(t, v.Info().Valid)
	assert.Equal(t, "free", v.Info().Tier)
}

func TestWatcher_NoChangeNoReload(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "license.key")

	exp := time.Now().Add(365 * 24 * time.Hour)
	token := signTestToken(t, priv, validClaims(exp))
	require.NoError(t, os.WriteFile(keyPath, token, 0600))

	v := NewValidator(pub, keyPath)
	v.LoadAndValidate()

	w := NewWatcher(v, keyPath)
	w.snapshot()

	// Check with no change — should not trigger reload.
	w.check()
	assert.True(t, v.Info().Valid)
}

func TestWatcher_StartContextCancel(t *testing.T) {
	pub, _ := testKeypair(t)
	v := NewValidator(pub, "/nonexistent/license.key")
	w := NewWatcher(v, "/nonexistent/license.key")
	// Override poll interval for test speed.
	w.pollInterval = 10 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Let it tick once.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK, Start returned.
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop after context cancel")
	}
}
