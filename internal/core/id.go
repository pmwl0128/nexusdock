package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	if prefix == "" {
		return hex.EncodeToString(raw), nil
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}
