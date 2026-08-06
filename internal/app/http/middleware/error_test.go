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
		{name: "invalid identity", err: oops.Wrap(domain.ErrInvalidIdentity), status: http.StatusUnauthorized, code: "invalid_identity", message: "a valid X-User-ID header is required"},
		{name: "invalid input", err: domain.ErrInvalidInput, status: http.StatusBadRequest, code: "invalid_input", message: "invalid request"},
		{name: "not found", err: domain.ErrNotFound, status: http.StatusNotFound, code: "not_found", message: "resource not found"},
		{name: "sold out", err: domain.ErrOutOfStock, status: http.StatusGone, code: "sold_out", message: "product is sold out"},
		{name: "queue full", err: domain.ErrQueueFull, status: http.StatusConflict, code: "queue_full", message: "queue is full"},
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
