package connector

import (
	"errors"
	"net/url"
)

func validateOutboundURL(rawURL string) error {
	const invalidOutboundURLError = "invalid outbound URL"

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New(invalidOutboundURLError)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return errors.New(invalidOutboundURLError)
	}
	return nil
}
