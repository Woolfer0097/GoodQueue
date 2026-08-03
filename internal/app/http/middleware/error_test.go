package middleware

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/samber/oops"
)

func TestMapError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{name: "oops wrapped not implemented", err: oops.Wrap(domain.ErrNotImplemented), status: http.StatusNotImplemented, code: "not_implemented", message: "not implemented"},
		{name: "invalid input", err: domain.ErrInvalidInput, status: http.StatusBadRequest, code: "invalid_input", message: "invalid request"},
		{name: "not found", err: domain.ErrNotFound, status: http.StatusNotFound, code: "not_found", message: "resource not found"},
		{name: "unknown", err: errors.New("database secret"), status: http.StatusInternalServerError, code: "internal_error", message: "internal server error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, response := MapError(test.err)
			if status != test.status || response.Error.Code != test.code || response.Error.Message != test.message {
				t.Fatalf("unexpected mapping: status=%d response=%+v", status, response)
			}
		})
	}
}
