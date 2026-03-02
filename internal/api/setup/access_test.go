package setup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSetupToken(t *testing.T) {
	t.Run("from env value", func(t *testing.T) {
		token, err := ResolveSetupToken("token-1", "")
		require.NoError(t, err)
		require.Equal(t, "token-1", token)
	})

	t.Run("from file", func(t *testing.T) {
		dir := t.TempDir()
		tokenPath := filepath.Join(dir, "setup.token")
		require.NoError(t, os.WriteFile(tokenPath, []byte("file-token\n"), 0o600))

		token, err := ResolveSetupToken("", tokenPath)
		require.NoError(t, err)
		require.Equal(t, "file-token", token)
	})
}

func TestParseAllowedCIDRs(t *testing.T) {
	values, err := ParseAllowedCIDRs([]string{"10.0.0.0/8", " 192.168.1.0/24 "})
	require.NoError(t, err)
	require.Len(t, values, 2)

	_, err = ParseAllowedCIDRs([]string{"invalid"})
	require.Error(t, err)
}

func TestAccessPolicyAuthorize(t *testing.T) {
	policy := AccessPolicy{
		TokenRequired: true,
		Token:         "setup-secret",
	}

	_, err := policy.Authorize("", "127.0.0.1")
	require.ErrorIs(t, err, ErrSetupTokenMissing)

	_, err = policy.Authorize("Bearer wrong", "127.0.0.1")
	require.ErrorIs(t, err, ErrSetupTokenInvalid)

	fingerprint, err := policy.Authorize("Bearer setup-secret", "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, fingerprint)
}
