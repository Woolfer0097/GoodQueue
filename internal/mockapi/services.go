package mockapi

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/mockdata"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

type ProductGetter interface {
	GetByID(context.Context, domain.ProductID) (*domain.Product, error)
}

type ProductService struct {
	productGetter ProductGetter
}

func NewProductService(productGetter ProductGetter) *ProductService {
	return &ProductService{productGetter: productGetter}
}

func (service *ProductService) List(context.Context) ([]domain.Product, error) {
	return mockdata.Products(), nil
}

func (service *ProductService) GetByID(ctx context.Context, productID domain.ProductID) (*domain.Product, error) {
	return service.productGetter.GetByID(ctx, productID)
}

type QueueService struct {
	status string
}

func NewQueueService(status string) *QueueService {
	return &QueueService{status: status}
}

func (service *QueueService) Join(_ context.Context, productID domain.ProductID, userID domain.ExternalUserID, _ uuid.UUID) (domain.QueueEntry, error) {
	if !mockdata.HasProduct(productID) {
		return domain.QueueEntry{}, domain.ErrProductNotFound
	}
	return mockdata.QueueEntry(mockdata.QueueStatusWaiting, productID, userID)
}

func (service *QueueService) Current(_ context.Context, productID domain.ProductID, userID domain.ExternalUserID) (domain.QueueEntry, error) {
	if !mockdata.HasProduct(productID) {
		return domain.QueueEntry{}, domain.ErrProductNotFound
	}
	return mockdata.QueueEntry(service.status, productID, userID)
}

func (service *QueueService) Leave(_ context.Context, productID domain.ProductID, userID domain.ExternalUserID) (domain.QueueEntry, error) {
	if !mockdata.HasProduct(productID) {
		return domain.QueueEntry{}, domain.ErrProductNotFound
	}
	return mockdata.QueueEntry(mockdata.QueueStatusCancelled, productID, userID)
}

type CheckoutService struct{}

func NewCheckoutService() *CheckoutService {
	return &CheckoutService{}
}

func (service *CheckoutService) Authorize(_ context.Context, productID domain.ProductID, _ domain.ExternalUserID) (domain.PurchaseRight, error) {
	if !mockdata.HasProduct(productID) {
		return domain.PurchaseRight{}, domain.ErrProductNotFound
	}
	return mockdata.CheckoutAuthorization(), nil
}
