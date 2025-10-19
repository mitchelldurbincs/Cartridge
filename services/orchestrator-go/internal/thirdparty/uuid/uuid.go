package uuid

import (
	"crypto/rand"
	"encoding/hex"
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
