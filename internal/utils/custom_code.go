package utils

import (
	"fmt"
	"regexp"
	"strings"
)

var customCodePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,20}$`)

var reservedShortCodes = map[string]struct{}{
	"health":  {},
	"stats":   {},
	"shorten": {},
}

func ValidateCustomCode(raw string) error {
	code := strings.TrimSpace(raw)
	if code == "" {
		return fmt.Errorf("custom code is empty")
	}

	if !customCodePattern.MatchString(code) {
		return fmt.Errorf("custom code must be 4-20 chars using letters, numbers, hyphens, or underscores")
	}

	return nil
}

func IsReservedShortCode(code string) bool {
	_, exists := reservedShortCodes[strings.ToLower(strings.TrimSpace(code))]
	return exists
}
