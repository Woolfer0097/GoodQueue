package domain

import (
	"fmt"
	"strings"
)

const maxExternalUserIDLength = 255

type ExternalUserID string

func ParseExternalUserID(raw string) (ExternalUserID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > maxExternalUserIDLength {
		return "", fmt.Errorf("%w: external user ID must contain 1..%d bytes", ErrInvalidInput, maxExternalUserIDLength)
	}
	return ExternalUserID(trimmed), nil
}
