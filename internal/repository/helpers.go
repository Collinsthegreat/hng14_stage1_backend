package repository

import "github.com/google/uuid"

// newUUIDv7 generates a new UUID v7 string.
func newUUIDv7() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
