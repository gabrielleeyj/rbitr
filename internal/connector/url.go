package connector

import (
	"context"
	"errors"
	"net"
	"net/url"
)

var (
	errInvalidOutboundURL = errors.New("invalid outbound URL")
	errBlockedPrivateIP   = errors.New("outbound URL resolves to a private or reserved IP address")

	// privateRanges contains CIDR blocks for private, loopback, link-local,
	// and cloud metadata IP ranges that should be blocked to prevent SSRF.
	privateRanges []*net.IPNet //nolint:gochecknoglobals // intentional SSRF block list initialized once
)

func init() {
	cidrs := []string{
		"127.0.0.0/8",    // loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // link-local (incl. cloud metadata 169.254.169.254)
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 unique local
		"fe80::/10",      // IPv6 link-local
	}
	for _, cidr := range cidrs {
		_, block, _ := net.ParseCIDR(cidr)
		privateRanges = append(privateRanges, block)
	}
}

func isPrivateIP(ip net.IP) bool {
	for _, block := range privateRanges {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// validateOutboundURL checks that a URL is valid for outbound requests.
// It validates scheme and host only — no DNS resolution, so it stays fast
// for the hot path in rest.go / mcp.go.
func validateOutboundURL(rawURL string) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errInvalidOutboundURL
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errInvalidOutboundURL
	}
	return nil
}

// ValidateToolBaseURL performs a thorough validation of a tool's base URL,
// including DNS resolution and private IP blocking. Use this at tool
// registration time (admin API), not on every request.
func ValidateToolBaseURL(rawURL string, allowPrivate bool) error {
	return validateToolBaseURLWithContext(context.Background(), rawURL, allowPrivate)
}

func validateToolBaseURLWithContext(ctx context.Context, rawURL string, allowPrivate bool) error {
	if err := validateOutboundURL(rawURL); err != nil {
		return err
	}
	if allowPrivate {
		return nil
	}

	parsed, _ := url.ParseRequestURI(rawURL)
	hostname := parsed.Hostname()

	// Check if hostname is a literal IP first (avoids DNS lookup).
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateIP(ip) {
			return errBlockedPrivateIP
		}
		return nil
	}

	// Resolve hostname and check all returned IPs.
	addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
	if err != nil {
		return errors.New("failed to resolve hostname: " + hostname)
	}
	for _, addr := range addrs {
		if ip := net.ParseIP(addr); ip != nil && isPrivateIP(ip) {
			return errBlockedPrivateIP
		}
	}

	return nil
}
