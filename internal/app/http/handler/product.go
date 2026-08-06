package handler

import (
	"context"
	"net/http"

	httpmiddleware "github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ProductService interface {
	List(context.Context) ([]domain.Product, error)
	Get(context.Context, domain.ProductID) (domain.Product, error)
}

type ProductHandler struct{ products ProductService }

type ProductResponse struct {
	ID                    string `json:"id" binding:"required" format:"uuid"`
	Title                 string `json:"title" binding:"required"`
	Description           string `json:"description" binding:"required"`
	ImageURL              string `json:"image_url" binding:"required" format:"uri"`
	QueueEnabled          bool   `json:"queue_enabled" binding:"required"`
	AllocatableStock      int32  `json:"allocatable_stock" binding:"required" minimum:"0"`
	Reserved              int32  `json:"reserved" binding:"required" minimum:"0"`
	FreeStock             int32  `json:"free_stock" binding:"required" minimum:"0"`
	WaitingCount          int64  `json:"waiting_count" binding:"required" minimum:"0"`
	WaitingBufferCapacity int64  `json:"waiting_buffer_capacity" binding:"required" minimum:"0"`
}

func NewProductHandler(products ProductService) *ProductHandler {
	return &ProductHandler{products: products}
}

// List godoc
//
//	@Summary	List products with live inventory and queue availability
//	@Tags		products
//	@Produce	json
//	@Success	200	{array}		ProductResponse
//	@Failure	500	{object}	middleware.ErrorResponse
//	@Router		/api/v1/products [get]
func (handler *ProductHandler) List(c *gin.Context) {
	products, err := handler.products.List(c.Request.Context())
	if err != nil {
		_ = c.Error(err)
		return
	}
	response := make([]ProductResponse, 0, len(products))
	for _, product := range products {
		response = append(response, mapProduct(product))
	}
	c.JSON(http.StatusOK, response)
}

// Get godoc
//
//	@Summary	Get a product with live inventory and queue availability
//	@Tags		products
//	@Produce	json
//	@Param		productID	path		string	true	"Product UUID"	format(uuid)
//	@Success	200			{object}	ProductResponse
//	@Failure	400			{object}	middleware.ErrorResponse
//	@Failure	404			{object}	middleware.ErrorResponse
//	@Failure	500			{object}	middleware.ErrorResponse
//	@Router		/api/v1/products/{productID} [get]
func (handler *ProductHandler) Get(c *gin.Context) {
	productID, err := domain.ParseProductID(c.Param("productID"))
	if err != nil {
		_ = c.Error(err)
		return
	}
	product, err := handler.products.Get(c.Request.Context(), productID)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, mapProduct(product))
}

func mapProduct(product domain.Product) ProductResponse {
	return ProductResponse{
		ID: uuid.UUID(product.ID).String(), Title: product.Title, Description: product.Description,
		ImageURL: product.ImageURL, QueueEnabled: product.QueueEnabled,
		AllocatableStock: product.AllocatableStock, Reserved: product.Reserved, FreeStock: product.FreeStock(),
		WaitingCount: product.WaitingCount, WaitingBufferCapacity: product.WaitingCapacity,
	}
}

var _ = httpmiddleware.ErrorResponse{}
