package domain

import "errors"

var (
	ErrNotImplemented = errors.New("not implemented")
	ErrInvalidInput   = errors.New("invalid input")
	ErrNotFound       = errors.New("not found")
	ErrInternal       = errors.New("internal error")
)
