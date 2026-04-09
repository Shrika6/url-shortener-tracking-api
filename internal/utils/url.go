package utils

import (
	"fmt"
	"net/url"
	"strings"
)

func ValidateURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("url is required")
	}

	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url must start with http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("url host is required")
	}

	return nil
}
