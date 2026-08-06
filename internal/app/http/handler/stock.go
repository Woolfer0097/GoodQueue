package handler

import (
	"context"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type StockService interface {
	Adjust(context.Context, domain.StockAdjustmentCommand) (domain.StockAdjustmentResult, error)
}

type StockHandler struct{ stock StockService }

type StockAdjustmentRequest struct {
	Delta             int64  `json:"delta" binding:"required" example:"5"`
	Reason            string `json:"reason" binding:"required" maxlength:"200"`
	ExternalReference string `json:"external_reference" binding:"required" maxlength:"200"`
}

type StockAdjustmentResponse struct {
	StockBefore int32 `json:"stock_before"`
	StockAfter  int32 `json:"stock_after"`
}

func NewStockHandler(stock StockService) *StockHandler { return &StockHandler{stock: stock} }

// Adjust godoc
//
//	@Summary	Apply an idempotent internal stock adjustment
//	@Tags		internal
//	@Accept		json
//	@Produce	json
//	@Param		Idempotency-Key	header		string					true	"Product-scoped adjustment key"	minlength(1)	maxlength(128)
//	@Param		productID		path		string					true	"Product UUID"					format(uuid)
//	@Param		request			body		StockAdjustmentRequest	true	"Adjustment payload"
//	@Success	200				{object}	StockAdjustmentResponse
//	@Failure	400,404,409,500	{object}	middleware.ErrorResponse
//	@Router		/internal/v1/products/{productID}/stock-adjustments [post]
func (handler *StockHandler) Adjust(c *gin.Context) {
	productID, err := domain.ParseProductID(c.Param("productID"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	key, err := domain.ParseIdempotencyKey(c.GetHeader("Idempotency-Key"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	var request StockAdjustmentRequest
	if err := decodeStrictJSON(c, &request); err != nil {
		_ = c.Error(err)
		return
	}
	command, err := domain.ParseStockAdjustmentCommand(
		productID, key, request.Delta, request.Reason, request.ExternalReference,
	)
	if err != nil {
		_ = c.Error(err)
		return
	}
	result, err := handler.stock.Adjust(c.Request.Context(), command)
	if len(result.ResponseBody) > 0 {
		c.Data(result.HTTPStatus, "application/json", result.ResponseBody)
		return
	}
	if err != nil {
		_ = c.Error(err)
		return
	}
	_ = c.Error(domain.ErrInternal)
}

var _ = httpmiddleware.ErrorResponse{}
