package idgen

import "github.com/google/uuid"

type Generator interface {
	New() string
}

type UUIDGenerator struct{}

func NewUUIDGenerator() UUIDGenerator {
	return UUIDGenerator{}
}

func (UUIDGenerator) New() string {
	return uuid.NewString()
}
