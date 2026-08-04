package domain

import "errors"

var (
	ErrNotImplemented = errors.New("not implemented")
	ErrNotFound       = errors.New("not found")
	ErrConflict       = errors.New("conflict")
	ErrInvalidInput   = errors.New("invalid input")
	ErrOutOfStock     = errors.New("out of stock")
	ErrInternal       = errors.New("internal error")
)
