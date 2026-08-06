package handler

import (
	"context"
	"net/http"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/app/identity"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type CheckoutService interface {
	Start(context.Context, domain.AttemptID, domain.ExternalUserID) (domain.QueueAttempt, error)
}

type CheckoutHandler struct{ checkout CheckoutService }

func NewCheckoutHandler(checkout CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{checkout: checkout}
}

// Start godoc
//
//	@Summary	Start or replay checkout for an invited queue attempt
//	@Tags		checkout
//	@Produce	json
//	@Param		X-User-ID				header		string	true	"Canonical lowercase external user UUID"	format(uuid)
//	@Param		attemptID				path		string	true	"Queue attempt UUID"						format(uuid)
//	@Success	200						{object}	QueueEntryResponse
//	@Failure	400,401,404,409,410,500	{object}	middleware.ErrorResponse
//	@Router		/api/v1/queue-attempts/{attemptID}/checkout [post]
func (handler *CheckoutHandler) Start(c *gin.Context) {
	userID, exists := identity.FromContext(c)
	if !exists {
		_ = c.Error(domain.ErrInvalidIdentity)
		return
	}
	attemptID, err := domain.ParseAttemptID(c.Param("attemptID"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	attempt, err := handler.checkout.Start(c.Request.Context(), attemptID, userID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, mapQueueAttempt(attempt, 0))
}

var _ = httpmiddleware.ErrorResponse{}
