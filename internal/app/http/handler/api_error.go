package handler

import (
	"errors"
	"net/http"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/app/identity"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Code      string `json:"code" binding:"required" example:"PRODUCT_NOT_FOUND"`
	Message   string `json:"message" binding:"required" example:"Товар не найден"`
	RequestID string `json:"request_id" binding:"required" example:"7ae799c1-0dfa-4248-b80b-4e60e61f431d"`
}

func parseProductID(c *gin.Context) (domain.ProductID, bool) {
	productID, err := domain.ParseProductID(c.Param("productID"))
	if err != nil {
		writeAPIError(c, http.StatusBadRequest, "INVALID_PRODUCT_ID", "Некорректный идентификатор товара")
		return domain.ProductID{}, false
	}
	return productID, true
}

func requireUserID(c *gin.Context) (domain.ExternalUserID, bool) {
	values, exists := c.Request.Header[http.CanonicalHeaderKey(identity.HeaderName)]
	if !exists {
		writeAPIError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Требуется идентификатор пользователя")
		return "", false
	}
	if len(values) != 1 {
		writeAPIError(c, http.StatusUnauthorized, "INVALID_USER_ID", "Некорректный идентификатор пользователя")
		return "", false
	}
	userID, err := domain.ParseExternalUserID(values[0])
	if err != nil {
		writeAPIError(c, http.StatusUnauthorized, "INVALID_USER_ID", "Некорректный идентификатор пользователя")
		return "", false
	}
	return userID, true
}

func handleAPIServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrProductNotFound):
		writeAPIError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Товар не найден")
	case errors.Is(err, domain.ErrNotImplemented),
		errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrNotFound),
		errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrOutOfStock),
		errors.Is(err, domain.ErrGrantExpired):
		_ = c.Error(err)
	default:
		_ = c.Error(err)
		writeAPIError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Внутренняя ошибка сервера")
	}
}

func writeAPIError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{
		Code:      code,
		Message:   message,
		RequestID: httpmiddleware.RequestID(c),
	})
}
