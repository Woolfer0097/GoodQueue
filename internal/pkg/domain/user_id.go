package domain

import (
	"fmt"

	"github.com/google/uuid"
)

const maxExternalUserIDLength = 255

type ExternalUserID string

func ParseExternalUserID(raw string) (ExternalUserID, error) {
	if raw == "" || len(raw) > maxExternalUserIDLength {
		return "", fmt.Errorf("%w: external user ID is required", ErrInvalidIdentity)
	}
	parsed, err := uuid.Parse(raw)
	if err != nil || parsed.String() != raw {
		return "", fmt.Errorf("%w: external user ID must be a canonical lowercase UUID", ErrInvalidIdentity)
	}
	return ExternalUserID(raw), nil
}
