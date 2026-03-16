package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// ProvenanceHeader is the header used for cross-tenant provenance chain tokens.
	ProvenanceHeader = "X-Provenance-Chain"

	// provenanceSigningKeyLen is the length of the HMAC signing key in bytes.
	provenanceSigningKeyLen = 32

	// provenanceTTL is the lifetime of a provenance token (short-lived, single hop).
	provenanceTTL = 30 * time.Second
)

// Provenance token errors.
var (
	ErrProvenanceExpired  = errors.New("provenance token expired")
	ErrProvenanceInvalid  = errors.New("provenance token invalid")
	ErrProvenanceKeyGen   = errors.New("failed to generate provenance signing key")
	ErrChainDepthExceeded = errors.New("cross-tenant chain depth exceeded")
)

// ProvenanceClaims are the claims encoded in a provenance chain token.
type ProvenanceClaims struct {
	SourceTenantID   string `json:"src"`
	SourceDecisionID string `json:"sdid"`
	ChainDepth       int    `json:"depth"`
	IssuedAt         int64  `json:"iat"`
	ExpiresAt        int64  `json:"exp"`
}

// ProvenanceManager creates and validates cross-tenant provenance chain tokens.
type ProvenanceManager struct {
	signingKey    []byte
	maxChainDepth int
}

// NewProvenanceManager creates a ProvenanceManager with a random signing key.
func NewProvenanceManager(maxChainDepth int) (*ProvenanceManager, error) {
	key := make([]byte, provenanceSigningKeyLen) //nolint:makezero // rand.Read requires pre-allocated slice
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrProvenanceKeyGen, err)
	}

	return &ProvenanceManager{
		signingKey:    key,
		maxChainDepth: maxChainDepth,
	}, nil
}

// newProvenanceManagerWithKey creates a ProvenanceManager with a specific key (for testing).
func newProvenanceManagerWithKey(key []byte, maxChainDepth int) *ProvenanceManager {
	return &ProvenanceManager{
		signingKey:    key,
		maxChainDepth: maxChainDepth,
	}
}

// IssueToken creates a provenance chain token for a cross-tenant request.
func (pm *ProvenanceManager) IssueToken(sourceTenantID, sourceDecisionID string, currentDepth int) (string, error) {
	nextDepth := currentDepth + 1
	if nextDepth > pm.maxChainDepth {
		return "", ErrChainDepthExceeded
	}

	now := time.Now()
	claims := ProvenanceClaims{
		SourceTenantID:   sourceTenantID,
		SourceDecisionID: sourceDecisionID,
		ChainDepth:       nextDepth,
		IssuedAt:         now.Unix(),
		ExpiresAt:        now.Add(provenanceTTL).Unix(),
	}

	return pm.signClaims(&claims)
}

// ValidateToken verifies a provenance chain token and returns the claims.
func (pm *ProvenanceManager) ValidateToken(token string) (ProvenanceClaims, error) {
	claims, err := pm.verifyClaims(token)
	if err != nil {
		return ProvenanceClaims{}, err
	}

	if time.Now().Unix() > claims.ExpiresAt {
		return ProvenanceClaims{}, ErrProvenanceExpired
	}

	if claims.ChainDepth > pm.maxChainDepth {
		return ProvenanceClaims{}, ErrChainDepthExceeded
	}

	return claims, nil
}

// MaxChainDepth returns the configured maximum chain depth.
func (pm *ProvenanceManager) MaxChainDepth() int {
	return pm.maxChainDepth
}

// ProvenanceFromRequest extracts a provenance token from the request header.
func ProvenanceFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(ProvenanceHeader))
}

func (pm *ProvenanceManager) signClaims(claims *ProvenanceClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal provenance claims: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature := pm.computeHMAC([]byte(encodedPayload))
	encodedSig := base64.RawURLEncoding.EncodeToString(signature)

	return encodedPayload + "." + encodedSig, nil
}

func (pm *ProvenanceManager) verifyClaims(token string) (ProvenanceClaims, error) {
	parts := strings.SplitN(token, ".", 2) //nolint:mnd // token format: payload.signature
	if len(parts) != 2 {                   //nolint:mnd // token format: payload.signature
		return ProvenanceClaims{}, ErrProvenanceInvalid
	}

	encodedPayload := parts[0]
	encodedSig := parts[1]

	expectedSig := pm.computeHMAC([]byte(encodedPayload))
	providedSig, err := base64.RawURLEncoding.DecodeString(encodedSig)
	if err != nil {
		return ProvenanceClaims{}, ErrProvenanceInvalid
	}
	if !hmac.Equal(expectedSig, providedSig) {
		return ProvenanceClaims{}, ErrProvenanceInvalid
	}

	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return ProvenanceClaims{}, ErrProvenanceInvalid
	}

	var claims ProvenanceClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ProvenanceClaims{}, ErrProvenanceInvalid
	}

	return claims, nil
}

func (pm *ProvenanceManager) computeHMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, pm.signingKey)
	mac.Write(data)
	return mac.Sum(nil)
}
