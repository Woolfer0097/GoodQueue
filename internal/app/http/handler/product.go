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
	GetByID(context.Context, domain.ProductID) (*domain.Product, error)
}

type ProductHandler struct{ products ProductService }

type ProductResponse struct {
	ID              string `json:"id" binding:"required" format:"uuid" example:"280f1230-81e3-4e10-aad6-864d8bb12a78"`
	Title           string `json:"title" binding:"required" example:"Лимитированная игровая приставка"`
	Description     string `json:"description" binding:"required" example:"Описание товара"`
	Price           int64  `json:"price" binding:"required" example:"1999900"`
	ImageURL        string `json:"image_url" binding:"required" format:"uri" example:"https://example.com/product.jpg"`
	Available       int    `json:"available" binding:"required" example:"1"`
	QueueEnabled    bool   `json:"queue_enabled" binding:"required" example:"true"`
	RightTTLSeconds int    `json:"right_ttl_seconds" binding:"required" example:"120"`
}

func NewProductHandler(products ProductService) *ProductHandler {
	return &ProductHandler{products: products}
}

// List godoc
//
//	@Summary		List products
//	@Description	Returns the mock product catalog when mock API mode is enabled.
//	@Tags			products
//	@Produce		json
//	@Success		200	{array}		ProductResponse
//	@Failure		500	{object}	ErrorResponse
//	@Failure		501	{object}	middleware.ErrorResponse
//	@Router			/api/v1/products [get]
func (handler *ProductHandler) List(c *gin.Context) {
	products, err := handler.products.List(c.Request.Context())
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}

	response := make([]ProductResponse, 0, len(products))
	for index := range products {
		response = append(response, productResponse(&products[index]))
	}
	c.JSON(http.StatusOK, response)
}

// Get godoc
//
//	@Summary		Get a product
//	@Description	Returns a product from PostgreSQL by UUID.
//	@Tags			products
//	@Produce		json
//	@Param			X-User-ID	header		string	false	"Trusted external user identity supplied by upstream authentication"	maxlength(255)
//	@Param			productID	path		string	true	"Product UUID"															format(uuid)
//	@Success		200			{object}	ProductResponse
//	@Failure		400			{object}	ErrorResponse	"INVALID_PRODUCT_ID"
//	@Failure		404			{object}	ErrorResponse	"PRODUCT_NOT_FOUND"
//	@Failure		500			{object}	ErrorResponse	"INTERNAL_ERROR"
//	@Router			/api/v1/products/{productID} [get]
func (handler *ProductHandler) Get(c *gin.Context) {
	productID, valid := parseProductID(c)
	if !valid {
		return
	}

	product, err := handler.products.GetByID(c.Request.Context(), productID)
	if err != nil {
		handleAPIServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, productResponse(product))
}

func productResponse(product *domain.Product) ProductResponse {
	return ProductResponse{
		ID:              uuid.UUID(product.ID).String(),
		Title:           product.Title,
		Description:     product.Description,
		Price:           product.PriceKopecks,
		ImageURL:        product.ImageURL,
		Available:       product.AllocatableStock,
		QueueEnabled:    product.QueueEnabled,
		RightTTLSeconds: product.RightTTLSeconds,
	}
}

var _ = httpmiddleware.ErrorResponse{}
