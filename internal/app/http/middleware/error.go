package middleware

import (
	"errors"
	"net/http"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ErrorBody struct {
	Code    string `json:"code" binding:"required" example:"not_implemented"`
	Message string `json:"message" binding:"required" example:"not implemented"`
}

type ErrorResponse struct {
	Error ErrorBody `json:"error" binding:"required"`
}

func ErrorHandler(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}

		status, response := MapError(c.Errors.Last().Err)
		if status >= http.StatusInternalServerError && status != http.StatusNotImplemented {
			log.Error("request failed", zap.Error(c.Errors.Last().Err), zap.String("request_id", RequestID(c)))
		}
		if c.Writer.Written() {
			return
		}
		c.AbortWithStatusJSON(status, response)
	}
}

func MapError(err error) (int, ErrorResponse) {
	switch {
	case errors.Is(err, domain.ErrNotImplemented):
		return http.StatusNotImplemented, ErrorResponse{Error: ErrorBody{Code: "not_implemented", Message: "not implemented"}}
	case errors.Is(err, domain.ErrInvalidInput):
		return http.StatusBadRequest, ErrorResponse{Error: ErrorBody{Code: "invalid_input", Message: "invalid request"}}
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, ErrorResponse{Error: ErrorBody{Code: "not_found", Message: "resource not found"}}
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, ErrorResponse{Error: ErrorBody{Code: "conflict", Message: "conflicting queue entry already exists"}}
	case errors.Is(err, domain.ErrOutOfStock):
		return http.StatusConflict, ErrorResponse{Error: ErrorBody{Code: "out_of_stock", Message: "no purchase slot is currently available"}}
	case errors.Is(err, domain.ErrGrantExpired):
		return http.StatusConflict, ErrorResponse{Error: ErrorBody{Code: "grant_expired", Message: "purchase right is not active"}}
	default:
		return http.StatusInternalServerError, ErrorResponse{Error: ErrorBody{Code: "internal_error", Message: "internal server error"}}
	}
}
