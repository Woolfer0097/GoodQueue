package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const testProductID = "280f1230-81e3-4e10-aad6-864d8bb12a78"

type productServiceStub struct {
	getByID func(context.Context, domain.ProductID) (*domain.Product, error)
}

func (stub productServiceStub) List(context.Context) ([]domain.Product, error) {
	return nil, domain.ErrNotImplemented
}

func (stub productServiceStub) GetByID(ctx context.Context, productID domain.ProductID) (*domain.Product, error) {
	return stub.getByID(ctx, productID)
}

func productTestRouter(service ProductService) http.Handler {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.RequestIDMiddleware(), middleware.ErrorHandler(zap.NewNop()))
	router.GET("/api/v1/products/:productID", NewProductHandler(service).Get)
	return router
}

func TestProductGetReturnsMappedResponse(t *testing.T) {
	expectedID := uuid.MustParse(testProductID)
	service := productServiceStub{getByID: func(_ context.Context, productID domain.ProductID) (*domain.Product, error) {
		if uuid.UUID(productID) != expectedID {
			t.Fatalf("product ID = %s, want %s", uuid.UUID(productID), expectedID)
		}
		return &domain.Product{
			ID:               domain.ProductID(expectedID),
			Title:            "Лимитированная игровая приставка",
			Description:      "Описание товара",
			PriceKopecks:     1999900,
			ImageURL:         "https://example.com/product.jpg",
			AllocatableStock: 1,
			QueueEnabled:     true,
			RightTTLSeconds:  120,
		}, nil
	}}

	recorder := httptest.NewRecorder()
	productTestRouter(service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/products/"+testProductID, nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response ProductResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.ID != testProductID || response.Title != "Лимитированная игровая приставка" || response.Description != "Описание товара" {
		t.Fatalf("identity fields were mapped incorrectly: %+v", response)
	}
	if response.Price != 1999900 {
		t.Fatalf("price = %d, want price_kopecks 1999900", response.Price)
	}
	if response.Available != 1 {
		t.Fatalf("available = %d, want allocatable_stock 1", response.Available)
	}
	if !response.QueueEnabled || response.RightTTLSeconds != 120 {
		t.Fatalf("queue fields were mapped incorrectly: %+v", response)
	}
}

func TestProductGetRejectsInvalidID(t *testing.T) {
	service := productServiceStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		t.Fatal("service called for invalid product ID")
		return nil, nil
	}}

	response := performProductErrorRequest(t, service, "not-a-uuid", http.StatusBadRequest)
	if response.Code != "INVALID_PRODUCT_ID" || response.Message != "Некорректный идентификатор товара" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestProductGetMapsNotFound(t *testing.T) {
	service := productServiceStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		return nil, domain.ErrProductNotFound
	}}

	response := performProductErrorRequest(t, service, testProductID, http.StatusNotFound)
	if response.Code != "PRODUCT_NOT_FOUND" || response.Message != "Товар не найден" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestProductGetHidesUnknownErrorAndReturnsRequestID(t *testing.T) {
	repository := productServiceStub{getByID: func(context.Context, domain.ProductID) (*domain.Product, error) {
		return nil, errors.New("postgres secret: SELECT * FROM products")
	}}
	service := usecase.NewProductUseCase(repository)

	response := performProductErrorRequest(t, service, testProductID, http.StatusInternalServerError)
	if response.Code != "INTERNAL_ERROR" || response.Message != "Внутренняя ошибка сервера" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.RequestID != "test-request-id" {
		t.Fatalf("request_id = %q, want test-request-id", response.RequestID)
	}
}

func performProductErrorRequest(t *testing.T, service ProductService, productID string, expectedStatus int) ErrorResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/products/"+productID, nil)
	request.Header.Set("X-Request-ID", "test-request-id")
	recorder := httptest.NewRecorder()
	productTestRouter(service).ServeHTTP(recorder, request)
	if recorder.Code != expectedStatus {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, expectedStatus, recorder.Body.String())
	}
	var response ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.RequestID == "" {
		t.Fatal("request_id is empty")
	}
	return response
}
