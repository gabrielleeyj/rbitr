package license

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

var (
	ErrInvalidPEM        = errors.New("invalid PEM block")
	ErrNotEd25519Public  = errors.New("not an Ed25519 public key")
	ErrNotEd25519Private = errors.New("not an Ed25519 private key")
)

// ParsePublicKeyPEM parses an Ed25519 public key from PEM-encoded bytes.
func ParsePublicKeyPEM(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEM
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing public key: %w", err)
	}

	edKey, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, ErrNotEd25519Public
	}

	return edKey, nil
}

// ParsePrivateKeyPEM parses an Ed25519 private key from PEM-encoded bytes.
func ParsePrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, ErrInvalidPEM
	}

	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing private key: %w", err)
	}

	edKey, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrNotEd25519Private
	}

	return edKey, nil
}

// MarshalPublicKeyPEM encodes an Ed25519 public key to PEM format.
func MarshalPublicKeyPEM(key ed25519.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshaling public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}), nil
}

// MarshalPrivateKeyPEM encodes an Ed25519 private key to PEM format.
func MarshalPrivateKeyPEM(key ed25519.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshaling private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}), nil
}
