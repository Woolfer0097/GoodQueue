package handler

import (
	"context"
	"time"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type CheckoutService interface {
	Authorize(context.Context, domain.ProductID, domain.ExternalUserID) (domain.PurchaseRight, error)
}

var _ = httpmiddleware.ErrorResponse{}

type CheckoutHandler struct{ checkout CheckoutService }

type CheckoutAuthorizationResponse struct {
	PurchaseRightID string                     `json:"purchase_right_id" binding:"required" format:"uuid"`
	QueueTicketID   int64                      `json:"queue_ticket_id" binding:"required" example:"42"`
	Status          domain.PurchaseRightStatus `json:"status" binding:"required" enums:"active,expired,consumed" example:"active"`
	IssuedAt        time.Time                  `json:"issued_at" binding:"required" format:"date-time"`
	ExpiresAt       time.Time                  `json:"expires_at" binding:"required" format:"date-time"`
}

func NewCheckoutHandler(checkout CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{checkout: checkout}
}

// Authorize godoc
//
//	@Summary		Authorize checkout
//	@Description	Future authorization uses the trusted external user ID, product ID, and an active database purchase right. No bearer purchase token is accepted. Currently returns 501 without querying or mutating PostgreSQL.
//	@Tags			checkout
//	@Produce		json
//	@Param			X-User-ID	header		string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"															format(uuid)
//	@Success		200			{object}	CheckoutAuthorizationResponse
//	@Failure		501			{object}	middleware.ErrorResponse
//	@Router			/api/v1/products/{productID}/checkout-authorizations [post]
func (handler *CheckoutHandler) Authorize(c *gin.Context) {
	_, err := handler.checkout.Authorize(c.Request.Context(), domain.ProductID{}, "")
	_ = c.Error(err)
}
