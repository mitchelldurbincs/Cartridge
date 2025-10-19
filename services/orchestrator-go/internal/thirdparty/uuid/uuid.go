package uuid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

type UUID [16]byte

func New() UUID {
	var id UUID
	if _, err := rand.Read(id[:]); err != nil {
		return id
	}
	return id
}

func (u UUID) String() string {
	return hex.EncodeToString(u[:])
}

func Parse(s string) (UUID, error) {
	var id UUID
	normalized := strings.ReplaceAll(s, "-", "")
	if len(normalized) != 32 {
		return id, fmt.Errorf("invalid UUID length: %d", len(normalized))
	}

	bytes, err := hex.DecodeString(normalized)
	if err != nil {
		return id, fmt.Errorf("invalid UUID format: %w", err)
	}

	copy(id[:], bytes)
	return id, nil
}

func MustParse(s string) UUID {
	id, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return id
}
