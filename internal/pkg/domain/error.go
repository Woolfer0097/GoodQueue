package domain

import "errors"

var (
	ErrNotImplemented  = errors.New("not implemented")
	ErrInvalidInput    = errors.New("invalid input")
	ErrNotFound        = errors.New("not found")
	ErrProductNotFound = errors.New("product not found")
	ErrConflict        = errors.New("conflict")
	ErrOutOfStock      = errors.New("out of stock")
	ErrGrantExpired    = errors.New("purchase right expired")
	ErrInternal        = errors.New("internal error")
)
