package repository

import (
	"context"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type Recommendation interface {
	ListEmbeddingDocuments(context.Context, string) ([]domain.ProductEmbeddingDocument, error)
	UpsertEmbeddings(context.Context, []domain.ProductEmbedding) error
	ListSemanticAlternatives(context.Context, domain.ProductID, string, int) ([]domain.ProductRecommendation, error)
	ListFallbackAlternatives(context.Context, domain.ProductID, int) ([]domain.ProductRecommendation, error)
}
