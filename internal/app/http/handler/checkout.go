package handler

import (
	"context"
	"net/http"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CheckoutService interface {
	Authorize(context.Context, domain.ProductID, domain.ExternalUserID) (domain.PurchaseRight, error)
}

type CheckoutHandler struct{ checkout CheckoutService }

type CheckoutAuthorizationResponse struct {
	Authorized      bool   `json:"authorized" binding:"required" example:"true"`
	AuthorizationID string `json:"authorization_id" binding:"required" format:"uuid"`
	EntryID         int64  `json:"entry_id" binding:"required" example:"42"`
	ProductID       string `json:"product_id" binding:"required" format:"uuid"`
	Status          string `json:"status" binding:"required" example:"purchased"`
	AuthorizedAt    string `json:"authorized_at" binding:"required" format:"date-time"`
}

func NewCheckoutHandler(checkout CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{checkout: checkout}
}

// Authorize godoc
//
//	@Summary		Authorize checkout
//	@Description	Returns a stable successful authorization in mock API mode; no purchase right or stock is changed.
//	@Tags			checkout
//	@Accept			json
//	@Produce		json
//	@Param			X-User-ID	header		string	true	"External user identity"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"				format(uuid)
//	@Success		200			{object}	CheckoutAuthorizationResponse
//	@Failure		400			{object}	ErrorResponse				"INVALID_PRODUCT_ID"
//	@Failure		401			{object}	ErrorResponse				"UNAUTHORIZED or INVALID_USER_ID"
//	@Failure		404			{object}	ErrorResponse				"PRODUCT_NOT_FOUND"
//	@Failure		500			{object}	ErrorResponse				"INTERNAL_ERROR"
//	@Failure		501			{object}	middleware.ErrorResponse	"Mock API disabled"
//	@Router			/api/v1/products/{productID}/checkout-authorizations [post]
func (handler *CheckoutHandler) Authorize(c *gin.Context) {
	productID, valid := parseProductID(c)
	if !valid {
		return
	}
	userID, valid := requireUserID(c)
	if !valid {
		return
	}

	authorization, err := handler.checkout.Authorize(c.Request.Context(), productID, userID)
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, CheckoutAuthorizationResponse{
		Authorized:      true,
		AuthorizationID: authorization.ID.String(),
		EntryID:         authorization.QueueTicketID,
		ProductID:       uuid.UUID(productID).String(),
		Status:          "purchased",
		AuthorizedAt:    authorization.IssuedAt.UTC().Format(time.RFC3339),
	})
}

var _ = httpmiddleware.ErrorResponse{}
