package license

import (
	"crypto/ed25519"
	_ "embed"
	"fmt"
)

//go:embed pubkey.pem
var embeddedPublicKeyPEM []byte

// EmbeddedPublicKey returns the Ed25519 public key embedded in the binary.
// This key is used to verify license key signatures. It is baked in at
// build time and never changes unless the signing keypair is rotated.
func EmbeddedPublicKey() (ed25519.PublicKey, error) {
	key, err := ParsePublicKeyPEM(embeddedPublicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("license: failed to parse embedded public key: %w", err)
	}
	return key, nil
}
