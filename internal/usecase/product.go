package usecase

import (
	"context"
	"fmt"
	"sync"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/pkg/repository"
)

type ProductUseCase struct {
	products        repository.Product
	recommendations repository.Recommendation
	embedder        EmbeddingProvider
	embeddingMu     sync.Mutex
}

func NewProductUseCase(
	products repository.Product,
	recommendations repository.Recommendation,
	embedder EmbeddingProvider,
) *ProductUseCase {
	return &ProductUseCase{products: products, recommendations: recommendations, embedder: embedder}
}

func (useCase *ProductUseCase) List(ctx context.Context) ([]domain.Product, error) {
	return useCase.products.List(ctx)
}

func (useCase *ProductUseCase) Get(ctx context.Context, id domain.ProductID) (domain.Product, error) {
	return useCase.products.Get(ctx, id)
}

func (useCase *ProductUseCase) Alternatives(ctx context.Context, id domain.ProductID) ([]domain.ProductRecommendation, error) {
	if _, err := useCase.products.Get(ctx, id); err != nil {
		return nil, err
	}

	const resultLimit = 4
	if useCase.embedder != nil {
		// Embedding failures must never make the sold-out journey unavailable.
		_ = useCase.refreshEmbeddings(ctx)
		semantic, err := useCase.recommendations.ListSemanticAlternatives(
			ctx,
			id,
			useCase.embedder.Model(),
			resultLimit,
		)
		if err == nil && len(semantic) > 0 {
			return semantic, nil
		}
	}

	return useCase.recommendations.ListFallbackAlternatives(ctx, id, resultLimit)
}

func (useCase *ProductUseCase) refreshEmbeddings(ctx context.Context) error {
	useCase.embeddingMu.Lock()
	defer useCase.embeddingMu.Unlock()
	lease, acquired, err := useCase.recommendations.TryAcquireEmbeddingRefresh(ctx, useCase.embedder.Model())
	if err != nil || !acquired {
		return err
	}
	defer func() { _ = lease.Release() }()

	documents, err := useCase.recommendations.ListEmbeddingDocuments(ctx, useCase.embedder.Model())
	if err != nil || len(documents) == 0 {
		return err
	}

	inputs := make([]string, len(documents))
	for index := range documents {
		inputs[index] = documents[index].Content
	}
	vectors, err := useCase.embedder.Embed(ctx, inputs)
	if err != nil {
		return fmt.Errorf("embed product catalog: %w", err)
	}
	if len(vectors) != len(documents) {
		return fmt.Errorf("embed product catalog: got %d vectors for %d documents", len(vectors), len(documents))
	}

	embeddings := make([]domain.ProductEmbedding, len(documents))
	for index := range documents {
		embeddings[index] = domain.ProductEmbedding{
			ProductID: documents[index].ProductID, Model: useCase.embedder.Model(),
			ContentHash: documents[index].ContentHash, Vector: vectors[index],
		}
	}
	if err := useCase.recommendations.UpsertEmbeddings(ctx, embeddings); err != nil {
		return fmt.Errorf("cache product embeddings: %w", err)
	}
	return nil
}
