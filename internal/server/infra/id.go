package infra

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

type RandomIDGenerator struct{}

func (RandomIDGenerator) NewID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}

	token := hex.EncodeToString(buf)
	if prefix == "" {
		return token
	}

	return strings.TrimSpace(prefix) + "_" + token
}
