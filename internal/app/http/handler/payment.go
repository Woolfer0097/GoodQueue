package handler

import (
	"context"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type PaymentService interface {
	Process(context.Context, string, string, string, string, string) (domain.PaymentResult, error)
}

type PaymentHandler struct{ payments PaymentService }

type PaymentEventRequest struct {
	Provider         string  `json:"provider" binding:"required" maxlength:"64"`
	EventID          string  `json:"event_id" binding:"required" maxlength:"200"`
	AttemptID        string  `json:"attempt_id" binding:"required" format:"uuid"`
	Outcome          string  `json:"outcome" binding:"required" enums:"succeeded,failed"`
	PaymentReference *string `json:"payment_reference" binding:"required" maxlength:"200"`
}

type PaymentEventResponse struct {
	Code string `json:"code" binding:"required"`
}

func NewPaymentHandler(payments PaymentService) *PaymentHandler {
	return &PaymentHandler{payments: payments}
}

// Process godoc
//
//	@Summary		Process an unsafe unauthenticated demo payment callback
//	@Description	Disabled by default. Runtime registration requires GOODQUEUE_UNSAFE_PAYMENT_CALLBACK=true; do not expose publicly.
//	@Tags			internal
//	@Accept			json
//	@Produce		json
//	@Param			request	body		PaymentEventRequest	true	"Payment provider event"
//	@Success		200		{object}	PaymentEventResponse
//	@Success		202		{object}	PaymentEventResponse
//	@Failure		400		{object}	middleware.ErrorResponse
//	@Failure		404,409	{object}	PaymentEventResponse
//	@Failure		500		{object}	middleware.ErrorResponse
//	@Router			/internal/v1/payment-events [post]
func (handler *PaymentHandler) Process(c *gin.Context) {
	var request PaymentEventRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		_ = c.Error(err)
		return
	}
	if request.PaymentReference == nil {
		_ = c.Error(domain.ErrInvalidInput)
		return
	}
	result, err := handler.payments.Process(
		c.Request.Context(), request.Provider, request.EventID, request.AttemptID,
		request.Outcome, *request.PaymentReference,
	)
	if err != nil {
		_ = c.Error(err)
		return
	}
	if result.HTTPStatus < 100 || len(result.ResponseBody) == 0 {
		_ = c.Error(domain.ErrInternal)
		return
	}
	c.Data(result.HTTPStatus, "application/json", result.ResponseBody)
}

var _ = httpmiddleware.ErrorResponse{}
