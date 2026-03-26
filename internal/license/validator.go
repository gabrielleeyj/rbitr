package license

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	// CurrentKeyVersion is the latest key format version this build supports.
	CurrentKeyVersion = 1

	// MinSupportedKeyVersion is the oldest key format this build will accept.
	MinSupportedKeyVersion = 1

	// ExpiryGracePeriod adds a buffer to handle clock skew on customer machines.
	ExpiryGracePeriod = 24 * time.Hour

	// issuerClaim is the expected value of the "iss" JWT claim.
	issuerClaim = "rbitr"
)

// Validation errors.
var (
	ErrNoLicenseKey           = errors.New("no license key found")
	ErrInvalidSignature       = errors.New("license key signature invalid")
	ErrLicenseExpired         = errors.New("license key expired")
	ErrKeyVersionTooOld       = errors.New("license key version too old")
	ErrKeyVersionTooNew       = errors.New("license key version too new")
	ErrInvalidIssuer          = errors.New("license key issuer invalid")
	ErrMalformedLicense       = errors.New("license key malformed")
	ErrMissingEntitlements    = errors.New("license key missing entitlements")
	ErrMissingLicensee        = errors.New("license key missing licensee info")
	ErrInvalidKeyVersionRange = errors.New("min supported key version exceeds current key version")
)

// licenseClaims holds the custom JWT claims for an rbitr license.
type licenseClaims struct {
	KeyVersion   int           `json:"key_version"`
	Tier         string        `json:"tier"`
	Entitlements *Entitlements `json:"entitlements"`
	Licensee     *licenseeLic  `json:"licensee"`
}

type licenseeLic struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Validator loads, verifies, and caches a license key.
type Validator struct {
	publicKey ed25519.PublicKey
	keyPath   string

	mu   sync.RWMutex
	info LicenseInfo
}

// NewValidator creates a license validator with the given Ed25519 public key
// and path to the license key file.
func NewValidator(publicKey ed25519.PublicKey, keyPath string) *Validator {
	return &Validator{
		publicKey: publicKey,
		keyPath:   keyPath,
		info: LicenseInfo{
			Valid:        false,
			Tier:         "free",
			Entitlements: FreeTierDefaults(),
		},
	}
}

// LoadAndValidate reads the license key file and validates it. If the file is
// missing or invalid, entitlements fall back to the free tier.
func (v *Validator) LoadAndValidate() {
	info, err := v.validateFile(v.keyPath)
	if err != nil {
		if !errors.Is(err, ErrNoLicenseKey) {
			log.Printf("license: validation failed: %v (falling back to free tier)", err)
		} else {
			log.Printf("license: no key found at %s (running in free tier)", v.keyPath)
		}
		v.mu.Lock()
		v.info = LicenseInfo{
			Valid:        false,
			Tier:         "free",
			Entitlements: FreeTierDefaults(),
		}
		v.mu.Unlock()
		return
	}

	v.mu.Lock()
	v.info = info
	v.mu.Unlock()
	log.Printf("license: valid (tier=%s licensee=%q expires=%s key_version=%d)",
		info.Tier, info.Licensee, info.ExpiresAt.Format(time.DateOnly), info.KeyVersion)
}

// KeyPath returns the configured license key file path.
func (v *Validator) KeyPath() string {
	return v.keyPath
}

// Info returns the current license info (thread-safe).
func (v *Validator) Info() LicenseInfo {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.info
}

// Entitlements returns the current resolved entitlements (thread-safe).
func (v *Validator) Entitlements() Entitlements {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.info.Entitlements
}

// Fingerprint returns the SHA-256 hex fingerprint of the raw license key bytes.
func Fingerprint(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// validateFile reads and validates a license key file.
func (v *Validator) validateFile(path string) (LicenseInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return LicenseInfo{}, ErrNoLicenseKey
		}
		return LicenseInfo{}, fmt.Errorf("reading license file: %w", err)
	}
	return v.ValidateBytes(data)
}

// ValidateBytes parses and validates a raw license key token.
func (v *Validator) ValidateBytes(data []byte) (LicenseInfo, error) {
	// Build the JWK from the Ed25519 public key.
	key, err := jwk.Import(v.publicKey)
	if err != nil {
		return LicenseInfo{}, fmt.Errorf("importing public key: %w", err)
	}

	// Parse, verify signature, and validate standard claims (exp, nbf, iat).
	tok, err := jwt.Parse(data,
		jwt.WithKey(jwa.EdDSA(), key),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(ExpiryGracePeriod),
	)
	if err != nil {
		// Distinguish expired keys from signature failures for clearer logging.
		errMsg := err.Error()
		if strings.Contains(errMsg, "exp") || strings.Contains(errMsg, "token is expired") || strings.Contains(errMsg, "nbf") {
			return LicenseInfo{}, fmt.Errorf("%w: %w", ErrLicenseExpired, err)
		}
		return LicenseInfo{}, ErrInvalidSignature
	}

	// Validate issuer.
	iss, ok := tok.Issuer()
	if !ok || iss != issuerClaim {
		return LicenseInfo{}, ErrInvalidIssuer
	}

	// Try flat claims (key_version, tier, entitlements, licensee at top level).
	return v.validateFlatClaims(tok)
}

// validateFlatClaims handles keys where custom fields are at the JWT top level.
func (v *Validator) validateFlatClaims(tok jwt.Token) (LicenseInfo, error) {
	var claims licenseClaims

	var kvFloat float64
	if err := tok.Get("key_version", &kvFloat); err == nil {
		claims.KeyVersion = int(kvFloat)
	}

	var tier string
	if err := tok.Get("tier", &tier); err == nil {
		claims.Tier = tier
	}

	var entMap map[string]any
	if err := tok.Get("entitlements", &entMap); err == nil {
		b, marshalErr := json.Marshal(entMap)
		if marshalErr != nil {
			return LicenseInfo{}, fmt.Errorf("%w: entitlements marshal: %w", ErrMalformedLicense, marshalErr)
		}
		var ent Entitlements
		if unmarshalErr := json.Unmarshal(b, &ent); unmarshalErr != nil {
			return LicenseInfo{}, fmt.Errorf("%w: entitlements decode: %w", ErrMalformedLicense, unmarshalErr)
		}
		claims.Entitlements = &ent
	}

	var licMap map[string]any
	if err := tok.Get("licensee", &licMap); err == nil {
		b, marshalErr := json.Marshal(licMap)
		if marshalErr != nil {
			return LicenseInfo{}, fmt.Errorf("%w: licensee marshal: %w", ErrMalformedLicense, marshalErr)
		}
		var lic licenseeLic
		if unmarshalErr := json.Unmarshal(b, &lic); unmarshalErr != nil {
			return LicenseInfo{}, fmt.Errorf("%w: licensee decode: %w", ErrMalformedLicense, unmarshalErr)
		}
		claims.Licensee = &lic
	}

	return v.buildInfo(tok, &claims)
}

// buildInfo validates claims and constructs the LicenseInfo.
func (v *Validator) buildInfo(tok jwt.Token, claims *licenseClaims) (LicenseInfo, error) {
	// Validate key version.
	if claims.KeyVersion < MinSupportedKeyVersion {
		return LicenseInfo{}, fmt.Errorf(
			"%w: got %d, minimum is %d — please contact support for a new key",
			ErrKeyVersionTooOld, claims.KeyVersion, MinSupportedKeyVersion)
	}
	if claims.KeyVersion > CurrentKeyVersion {
		return LicenseInfo{}, fmt.Errorf(
			"%w: got %d, maximum is %d — please upgrade rbitr",
			ErrKeyVersionTooNew, claims.KeyVersion, CurrentKeyVersion)
	}

	// Validate tier.
	if claims.Tier != "free" && claims.Tier != "paid" && claims.Tier != "trial" {
		return LicenseInfo{}, fmt.Errorf("%w: unknown tier %q", ErrMalformedLicense, claims.Tier)
	}

	// Expiry is already validated by jwt.Parse with WithAcceptableSkew.
	// Extract exp for the LicenseInfo struct.
	exp, ok := tok.Expiration()
	if !ok {
		return LicenseInfo{}, fmt.Errorf("%w: missing expiration", ErrMalformedLicense)
	}

	// Validate licensee.
	if claims.Licensee == nil || claims.Licensee.Name == "" {
		return LicenseInfo{}, ErrMissingLicensee
	}

	// Resolve entitlements via merge-over-defaults.
	entitlements := MergeOverDefaults(claims.Tier, claims.Entitlements)

	iat, _ := tok.IssuedAt()
	sub, _ := tok.Subject()

	info := LicenseInfo{
		Valid:        true,
		Tier:         claims.Tier,
		KeyVersion:   claims.KeyVersion,
		Licensee:     sub,
		Email:        "",
		IssuedAt:     iat,
		ExpiresAt:    exp,
		Entitlements: entitlements,
	}

	if claims.Licensee != nil {
		info.Licensee = claims.Licensee.Name
		info.Email = claims.Licensee.Email
	}

	return info, nil
}
