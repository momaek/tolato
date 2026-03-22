package domain

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrAlreadyExists     = errors.New("already exists")
	ErrInvalidArgument   = errors.New("invalid argument")
	ErrRevisionConflict  = errors.New("revision conflict")
	ErrDuplicateAction   = errors.New("duplicate action")
	ErrSessionBusy       = errors.New("session busy")
	ErrUnsupportedConfig = errors.New("unsupported config")
)
