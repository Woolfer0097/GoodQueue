package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
)

type productRepositoryStub struct {
	getErr            error
	alternatives      []domain.Product
	alternativesCalls int
	limit             int
}

func (stub *productRepositoryStub) List(context.Context) ([]domain.Product, error) {
	return nil, nil
}

func (stub *productRepositoryStub) Get(context.Context, domain.ProductID) (domain.Product, error) {
	return domain.Product{}, stub.getErr
}

func (stub *productRepositoryStub) ListAvailableAlternatives(
	_ context.Context,
	_ domain.ProductID,
	limit int,
) ([]domain.Product, error) {
	stub.alternativesCalls++
	stub.limit = limit
	return stub.alternatives, nil
}

func TestProductAlternativesRequireExistingSourceProduct(t *testing.T) {
	repository := &productRepositoryStub{getErr: domain.ErrNotFound}
	_, err := NewProductUseCase(repository).Alternatives(context.Background(), domain.ProductID{})
	if !errors.Is(err, domain.ErrNotFound) || repository.alternativesCalls != 0 {
		t.Fatalf("missing source product: calls=%d err=%v", repository.alternativesCalls, err)
	}
}

func TestProductAlternativesUseBoundedMVPResult(t *testing.T) {
	want := []domain.Product{{Title: "alternative"}}
	repository := &productRepositoryStub{alternatives: want}
	got, err := NewProductUseCase(repository).Alternatives(context.Background(), domain.ProductID{})
	if err != nil || len(got) != 1 || got[0].Title != want[0].Title {
		t.Fatalf("alternatives result: %#v err=%v", got, err)
	}
	if repository.alternativesCalls != 1 || repository.limit != 4 {
		t.Fatalf("alternatives query: calls=%d limit=%d", repository.alternativesCalls, repository.limit)
	}
}
