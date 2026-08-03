package handler

import (
	"context"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
)

type ProductService interface {
	List(context.Context) ([]domain.Product, error)
	Get(context.Context, domain.ProductID) (domain.Product, error)
}

type ProductHandler struct{ products ProductService }

type ProductResponse struct {
	ID               string `json:"id" binding:"required" format:"uuid" example:"f3cfd11c-a3d1-4ae4-a8b1-d3bc2e891bc7"`
	Title            string `json:"title" binding:"required" example:"Limited Edition Item"`
	Description      string `json:"description" binding:"required" example:"A scarce product offered through a fair queue."`
	ImageURL         string `json:"image_url" binding:"required" format:"uri" example:"https://example.invalid/product.jpg"`
	QueueEnabled     bool   `json:"queue_enabled" binding:"required" example:"true"`
	AllocatableStock int    `json:"allocatable_stock" binding:"required" minimum:"0" example:"10"`
	RightTTLSeconds  int    `json:"right_ttl_seconds" binding:"required" minimum:"30" maximum:"86400" example:"600"`
}

func NewProductHandler(products ProductService) *ProductHandler {
	return &ProductHandler{products: products}
}

// List godoc
//
//	@Summary		List products
//	@Description	Reserved business contract; returns 501 until product listing is implemented.
//	@Tags			products
//	@Produce		json
//	@Param			X-User-ID	header		string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Success		200			{array}		ProductResponse
//	@Failure		501			{object}	middleware.ErrorResponse
//	@Router			/api/v1/products [get]
func (handler *ProductHandler) List(c *gin.Context) {
	_, err := handler.products.List(c.Request.Context())
	_ = c.Error(err)
}

// Get godoc
//
//	@Summary		Get a product
//	@Description	Reserved business contract; returns 501 until product lookup is implemented.
//	@Tags			products
//	@Produce		json
//	@Param			X-User-ID	header		string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"															format(uuid)
//	@Success		200			{object}	ProductResponse
//	@Failure		501			{object}	middleware.ErrorResponse
//	@Router			/api/v1/products/{productID} [get]
func (handler *ProductHandler) Get(c *gin.Context) {
	_, err := handler.products.Get(c.Request.Context(), domain.ProductID{})
	_ = c.Error(err)
}

var _ = httpmiddleware.ErrorResponse{}
