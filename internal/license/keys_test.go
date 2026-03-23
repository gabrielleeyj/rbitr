package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalParsePublicKeyPEM(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pem, err := MarshalPublicKeyPEM(pub)
	require.NoError(t, err)

	parsed, err := ParsePublicKeyPEM(pem)
	require.NoError(t, err)
	assert.Equal(t, pub, parsed)
}

func TestMarshalParsePrivateKeyPEM(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pem, err := MarshalPrivateKeyPEM(priv)
	require.NoError(t, err)

	parsed, err := ParsePrivateKeyPEM(pem)
	require.NoError(t, err)
	assert.Equal(t, priv, parsed)
}

func TestParsePublicKeyPEM_InvalidPEM(t *testing.T) {
	_, err := ParsePublicKeyPEM([]byte("not-pem-data"))
	assert.ErrorIs(t, err, ErrInvalidPEM)
}

func TestParsePrivateKeyPEM_InvalidPEM(t *testing.T) {
	_, err := ParsePrivateKeyPEM([]byte("not-pem-data"))
	assert.ErrorIs(t, err, ErrInvalidPEM)
}
