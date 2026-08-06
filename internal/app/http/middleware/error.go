package middleware

import (
	"errors"
	"net/http"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ErrorBody struct {
	Code    string `json:"code" binding:"required" example:"invalid_input"`
	Message string `json:"message" binding:"required" example:"invalid request"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error" binding:"required"`
}

func ErrorHandler(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}

		status, response := MapError(c.Errors.Last().Err)
		if status >= http.StatusInternalServerError {
			log.Error("request failed", zap.Error(c.Errors.Last().Err), zap.String("request_id", RequestID(c)))
		}
		c.AbortWithStatusJSON(status, response)
	}
}

func MapError(err error) (int, ErrorResponse) {
	switch {
	case errors.Is(err, domain.ErrInvalidIdentity):
		return publicError(http.StatusUnauthorized, "invalid_identity", "a valid X-User-ID header is required")
	case errors.Is(err, domain.ErrInvalidInput):
		return publicError(http.StatusBadRequest, "invalid_input", "invalid request")
	case errors.Is(err, domain.ErrAttemptNotFound), errors.Is(err, domain.ErrNotFound):
		return publicError(http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, domain.ErrQueueDisabled):
		return publicError(http.StatusConflict, "queue_disabled", "queue is disabled")
	case errors.Is(err, domain.ErrQueueFull):
		return publicError(http.StatusConflict, "queue_full", "queue is full")
	case errors.Is(err, domain.ErrAlreadyPurchased):
		return publicError(http.StatusConflict, "already_purchased", "product was already purchased")
	case errors.Is(err, domain.ErrInvalidTransition), errors.Is(err, domain.ErrPaymentFailed):
		return publicError(http.StatusConflict, "invalid_transition", "operation is not valid for the current state")
	case errors.Is(err, domain.ErrAdjustmentConflict), errors.Is(err, domain.ErrConflict):
		return publicError(http.StatusConflict, "idempotency_conflict", "idempotency key was used with a different request")
	case errors.Is(err, domain.ErrOutOfStock):
		return publicError(http.StatusGone, "sold_out", "product is sold out")
	case errors.Is(err, domain.ErrAttemptGone), errors.Is(err, domain.ErrInvitationExpired), errors.Is(err, domain.ErrCheckoutExpired):
		return publicError(http.StatusGone, "expired", "queue attempt is no longer available")
	default:
		return publicError(http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func publicError(status int, code, message string) (int, ErrorResponse) {
	return status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}}
}
