package setup

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/gabrielleeyj/rbitr/internal/utils"
)

var (
	ErrSetupTokenMissing    = errors.New("setup token missing")
	ErrSetupTokenInvalid    = errors.New("setup token invalid")
	ErrSetupNetworkRejected = errors.New("setup network rejected")
)

const idempotencyHeader = "Idempotency-Key"

const setupTokenFingerprintLength = 16

type AccessPolicy struct {
	TokenRequired bool
	Token         string
	AllowedCIDRs  []netip.Prefix
}

func ResolveSetupToken(rawToken, tokenFile string) (string, error) {
	if strings.TrimSpace(tokenFile) == "" {
		return strings.TrimSpace(rawToken), nil
	}
	data, err := os.ReadFile(strings.TrimSpace(tokenFile))
	if err != nil {
		return "", fmt.Errorf("read setup token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func ParseAllowedCIDRs(values []string) ([]netip.Prefix, error) {
	if len(values) == 0 {
		return nil, nil
	}
	allowed := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid RBTR_SETUP_ALLOWED_CIDRS value %q: %w", trimmed, err)
		}
		allowed = append(allowed, prefix.Masked())
	}
	return allowed, nil
}

func (p AccessPolicy) Authorize(authHeader, clientIP string) (string, error) {
	if !clientIPAllowed(clientIP, p.AllowedCIDRs) {
		return "", ErrSetupNetworkRejected
	}
	if !p.TokenRequired {
		return "", nil
	}
	token := bearerToken(authHeader)
	if token == "" {
		return "", ErrSetupTokenMissing
	}
	expected := strings.TrimSpace(p.Token)
	if expected == "" {
		return "", ErrSetupTokenInvalid
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		return "", ErrSetupTokenInvalid
	}
	return setupTokenFingerprint(token), nil
}

func setupTokenFingerprint(token string) string {
	sum := utils.HashString(strings.TrimSpace(token))
	if len(sum) > setupTokenFingerprintLength {
		sum = sum[:setupTokenFingerprintLength]
	}
	return "stf_" + sum
}

func clientIPAllowed(clientIP string, allowed []netip.Prefix) bool {
	if len(allowed) == 0 {
		return true
	}
	ip := strings.TrimSpace(clientIP)
	if ip == "" {
		return false
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		host, _, splitErr := net.SplitHostPort(ip)
		if splitErr != nil {
			return false
		}
		addr, err = netip.ParseAddr(host)
		if err != nil {
			return false
		}
	}
	for _, prefix := range allowed {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

//nolint:mnd // 2 is split segments.
func bearerToken(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
