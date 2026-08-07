package mockapi

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

const (
	ProductScarceID   = "11111111-1111-1111-1111-111111111111"
	ProductPopularID  = "22222222-2222-2222-2222-222222222222"
	ProductSoldOutID  = "33333333-3333-3333-3333-333333333333"
	ProductDisabledID = "44444444-4444-4444-4444-444444444444"

	DemoUserOneID   = "00000000-0000-4000-8000-000000000001"
	DemoUserTwoID   = "00000000-0000-4000-8000-000000000002"
	DemoUserThreeID = "00000000-0000-4000-8000-000000000003"
	DemoUserFourID  = "00000000-0000-4000-8000-000000000004"
	DemoUserFiveID  = "00000000-0000-4000-8000-000000000005"
)

type State struct {
	mu            sync.RWMutex
	products      map[domain.ProductID]*domain.Product
	productOrder  []domain.ProductID
	attempts      map[domain.ProductID][]*domain.QueueAttempt
	users         []domain.DemoUser
	invitationTTL time.Duration
	checkoutTTL   time.Duration
}

type Services struct {
	Products  *MockProductService
	Queue     *MockQueueService
	Checkout  *MockCheckoutService
	DemoUsers *MockDemoUserService
	State     *State
}

type MockProductService struct{ state *State }
type MockQueueService struct{ state *State }
type MockCheckoutService struct{ state *State }
type MockDemoUserService struct{ state *State }

func NewServices(invitationTTL, checkoutTTL time.Duration) Services {
	state := newState(time.Now().UTC(), invitationTTL, checkoutTTL)
	return Services{
		Products:  &MockProductService{state: state},
		Queue:     &MockQueueService{state: state},
		Checkout:  &MockCheckoutService{state: state},
		DemoUsers: &MockDemoUserService{state: state},
		State:     state,
	}
}

func newState(now time.Time, invitationTTL, checkoutTTL time.Duration) *State {
	productIDs := []domain.ProductID{
		productID(ProductScarceID), productID(ProductPopularID), productID(ProductSoldOutID), productID(ProductDisabledID),
	}
	products := []*domain.Product{
		{ID: productIDs[0], Title: "Дефицитный товар (mock)", Description: "Один checkout и два пользователя в очереди", ImageURL: "https://placehold.co/600x400?text=Scarce+Mock", Category: "collectibles", PriceCents: 1499000, QueueEnabled: true, AllocatableStock: 1, Reserved: 1, NextQueueSequence: 4, WaitingCapacity: 10},
		{ID: productIDs[1], Title: "Сценарии статусов (mock)", Description: "Готовые invited и terminal состояния", ImageURL: "https://placehold.co/600x400?text=Status+Mock", Category: "collectibles", PriceCents: 1299000, QueueEnabled: true, AllocatableStock: 3, Reserved: 1, NextQueueSequence: 6, WaitingCapacity: 10},
		{ID: productIDs[2], Title: "Раскупленный товар (mock)", Description: "Join возвращает sold_out", ImageURL: "https://placehold.co/600x400?text=Sold+Out", Category: "collectibles", PriceCents: 1599000, QueueEnabled: true, AllocatableStock: 0, NextQueueSequence: 1},
		{ID: productIDs[3], Title: "Очередь выключена (mock)", Description: "Join возвращает queue_disabled", ImageURL: "https://placehold.co/600x400?text=Queue+Disabled", Category: "other", PriceCents: 990000, QueueEnabled: false, AllocatableStock: 10, NextQueueSequence: 1, WaitingCapacity: 10},
	}
	productMap := make(map[domain.ProductID]*domain.Product, len(products))
	for _, product := range products {
		productMap[product.ID] = product
	}

	users := []domain.DemoUser{
		{ExternalUserID: userID(DemoUserOneID), DisplayName: "Пользователь 1"},
		{ExternalUserID: userID(DemoUserTwoID), DisplayName: "Пользователь 2"},
		{ExternalUserID: userID(DemoUserThreeID), DisplayName: "Пользователь 3"},
		{ExternalUserID: userID(DemoUserFourID), DisplayName: "Пользователь 4"},
		{ExternalUserID: userID(DemoUserFiveID), DisplayName: "Пользователь 5"},
	}

	state := &State{
		products: productMap, productOrder: productIDs, attempts: make(map[domain.ProductID][]*domain.QueueAttempt),
		users: users, invitationTTL: invitationTTL, checkoutTTL: checkoutTTL,
	}
	state.attempts[productIDs[0]] = []*domain.QueueAttempt{
		fixtureAttempt("a1111111-1111-4111-8111-111111111111", productIDs[0], users[0].ExternalUserID, 1, domain.QueueAttemptCheckout, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a1111111-1111-4111-8111-111111111112", productIDs[0], users[1].ExternalUserID, 2, domain.QueueAttemptWaiting, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a1111111-1111-4111-8111-111111111113", productIDs[0], users[2].ExternalUserID, 3, domain.QueueAttemptWaiting, now, invitationTTL, checkoutTTL),
	}
	state.attempts[productIDs[1]] = []*domain.QueueAttempt{
		fixtureAttempt("a2222222-2222-4222-8222-222222222221", productIDs[1], users[0].ExternalUserID, 1, domain.QueueAttemptInvited, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a2222222-2222-4222-8222-222222222222", productIDs[1], users[1].ExternalUserID, 2, domain.QueueAttemptPurchased, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a2222222-2222-4222-8222-222222222223", productIDs[1], users[2].ExternalUserID, 3, domain.QueueAttemptCancelled, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a2222222-2222-4222-8222-222222222224", productIDs[1], users[3].ExternalUserID, 4, domain.QueueAttemptInviteExpired, now, invitationTTL, checkoutTTL),
		fixtureAttempt("a2222222-2222-4222-8222-222222222225", productIDs[1], users[4].ExternalUserID, 5, domain.QueueAttemptCheckoutExpired, now, invitationTTL, checkoutTTL),
	}
	return state
}

func fixtureAttempt(rawID string, product domain.ProductID, user domain.ExternalUserID, sequence int64, state domain.QueueAttemptState, now time.Time, invitationTTL, checkoutTTL time.Duration) *domain.QueueAttempt {
	created := now.Add(-invitationTTL - checkoutTTL - 5*time.Minute)
	attempt := &domain.QueueAttempt{
		ID: domain.AttemptID(uuid.MustParse(rawID)), ProductID: product, QueueSequence: sequence,
		ExternalUserID: user, IdempotencyKey: domain.IdempotencyKey("fixture-" + rawID),
		State: state, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
	switch state {
	case domain.QueueAttemptInvited:
		attempt.InvitedAt = timePointer(now)
		attempt.InvitationDeadline = timePointer(now.Add(invitationTTL))
		attempt.UpdatedAt = now
	case domain.QueueAttemptCheckout:
		attempt.InvitedAt = timePointer(now)
		attempt.InvitationDeadline = timePointer(now.Add(invitationTTL))
		attempt.CheckoutStartedAt = timePointer(now)
		attempt.CheckoutDeadline = timePointer(now.Add(checkoutTTL))
		attempt.UpdatedAt = now
	case domain.QueueAttemptPurchased:
		terminal := now.Add(-time.Minute)
		attempt.TerminalAt, attempt.PurchasedAt = timePointer(terminal), timePointer(terminal)
		attempt.UpdatedAt = terminal
	case domain.QueueAttemptCancelled:
		terminal := now.Add(-time.Minute)
		attempt.TerminalAt = timePointer(terminal)
		attempt.UpdatedAt = terminal
	case domain.QueueAttemptInviteExpired:
		invited := now.Add(-invitationTTL - time.Minute)
		deadline := invited.Add(invitationTTL)
		attempt.InvitedAt, attempt.InvitationDeadline = timePointer(invited), timePointer(deadline)
		attempt.TerminalAt = timePointer(deadline)
		attempt.UpdatedAt = deadline
	case domain.QueueAttemptCheckoutExpired:
		started := now.Add(-checkoutTTL - time.Minute)
		attempt.InvitedAt = timePointer(started.Add(-time.Minute))
		attempt.InvitationDeadline = timePointer(started.Add(time.Minute))
		attempt.CheckoutStartedAt, attempt.CheckoutDeadline = timePointer(started), timePointer(started.Add(checkoutTTL))
		attempt.TerminalAt = timePointer(started.Add(checkoutTTL))
		attempt.UpdatedAt = *attempt.TerminalAt
	}
	return attempt
}

func (service *MockProductService) List(context.Context) ([]domain.Product, error) {
	service.state.mu.RLock()
	defer service.state.mu.RUnlock()
	return service.state.productsLocked(), nil
}

func (service *MockProductService) Get(_ context.Context, id domain.ProductID) (domain.Product, error) {
	service.state.mu.RLock()
	defer service.state.mu.RUnlock()
	return service.state.productLocked(id)
}

func (service *MockProductService) Alternatives(
	_ context.Context,
	id domain.ProductID,
) ([]domain.ProductRecommendation, error) {
	service.state.mu.RLock()
	defer service.state.mu.RUnlock()
	source, exists := service.state.products[id]
	if !exists {
		return nil, domain.ErrNotFound
	}
	products := make([]domain.Product, 0, len(service.state.products)-1)
	for _, candidate := range service.state.productsLocked() {
		if candidate.ID != id && candidate.QueueEnabled && candidate.FreeStock() > 0 {
			products = append(products, candidate)
		}
	}
	sort.Slice(products, func(i, j int) bool {
		left, right := products[i].FreeStock(), products[j].FreeStock()
		if left != right {
			return left > right
		}
		return uuid.UUID(products[i].ID).String() < uuid.UUID(products[j].ID).String()
	})
	if len(products) > 4 {
		products = products[:4]
	}
	recommendations := make([]domain.ProductRecommendation, 0, len(products))
	for _, product := range products {
		score := 0.5
		reason := domain.RecommendationReasonAvailable
		if product.Category == source.Category {
			score = 0.9
			reason = domain.RecommendationReasonSameCategory
		}
		recommendations = append(recommendations, domain.ProductRecommendation{
			Product: product, Score: score, Mode: domain.RecommendationModeFallback, ReasonCode: reason,
		})
	}
	return recommendations, nil
}

func (service *MockDemoUserService) List(context.Context) ([]domain.DemoUser, error) {
	service.state.mu.RLock()
	defer service.state.mu.RUnlock()
	return append([]domain.DemoUser(nil), service.state.users...), nil
}

func (service *MockQueueService) Join(_ context.Context, productID domain.ProductID, externalUserID domain.ExternalUserID, key domain.IdempotencyKey) (domain.JoinQueueResult, error) {
	state := service.state
	state.mu.Lock()
	defer state.mu.Unlock()
	product, exists := state.products[productID]
	if !exists {
		return domain.JoinQueueResult{}, domain.ErrNotFound
	}
	for _, attempt := range state.attempts[productID] {
		if attempt.ExternalUserID == externalUserID && attempt.IdempotencyKey == key {
			return state.joinResultLocked(*attempt, false), nil
		}
	}
	for _, attempt := range state.attempts[productID] {
		if attempt.ExternalUserID == externalUserID && attempt.State == domain.QueueAttemptPurchased {
			return domain.JoinQueueResult{}, domain.ErrAlreadyPurchased
		}
	}
	if active := state.activeAttemptLocked(productID, externalUserID); active != nil {
		return state.joinResultLocked(*active, false), nil
	}
	if !product.QueueEnabled {
		return domain.JoinQueueResult{}, domain.ErrQueueDisabled
	}
	if product.AllocatableStock == 0 {
		return domain.JoinQueueResult{}, domain.ErrOutOfStock
	}
	if product.FreeStock() == 0 && state.waitingCountLocked(productID) >= product.WaitingCapacity {
		return domain.JoinQueueResult{}, domain.ErrQueueFull
	}

	now := time.Now().UTC()
	attempt := &domain.QueueAttempt{
		ID: domain.AttemptID(uuid.New()), ProductID: productID, QueueSequence: product.NextQueueSequence,
		ExternalUserID: externalUserID, IdempotencyKey: key, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	product.NextQueueSequence++
	if product.FreeStock() > 0 {
		attempt.State = domain.QueueAttemptCheckout
		attempt.InvitedAt, attempt.InvitationDeadline = timePointer(now), timePointer(now.Add(state.invitationTTL))
		attempt.CheckoutStartedAt, attempt.CheckoutDeadline = timePointer(now), timePointer(now.Add(state.checkoutTTL))
		product.Reserved++
	} else {
		attempt.State = domain.QueueAttemptWaiting
	}
	state.attempts[productID] = append(state.attempts[productID], attempt)
	return state.joinResultLocked(*attempt, true), nil
}

func (service *MockQueueService) Current(_ context.Context, productID domain.ProductID, externalUserID domain.ExternalUserID) (domain.CurrentQueueResult, error) {
	state := service.state
	state.mu.RLock()
	defer state.mu.RUnlock()
	if _, exists := state.products[productID]; !exists {
		return domain.CurrentQueueResult{}, domain.ErrNotFound
	}
	attempt := state.activeAttemptLocked(productID, externalUserID)
	if attempt == nil {
		attempt = state.latestAttemptLocked(productID, externalUserID)
	}
	if attempt == nil {
		return domain.CurrentQueueResult{}, domain.ErrAttemptNotFound
	}
	return state.currentResultLocked(*attempt), nil
}

func (service *MockQueueService) Leave(_ context.Context, productID domain.ProductID, externalUserID domain.ExternalUserID) error {
	state := service.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, exists := state.products[productID]; !exists {
		return domain.ErrNotFound
	}
	attempt := state.latestAttemptLocked(productID, externalUserID)
	if attempt == nil {
		return domain.ErrAttemptNotFound
	}
	if attempt.State == domain.QueueAttemptPurchased {
		return domain.ErrAlreadyPurchased
	}
	if !isActive(attempt.State) {
		return nil
	}
	if reservesStock(attempt.State) {
		state.products[productID].Reserved--
	}
	now := time.Now().UTC()
	attempt.State, attempt.TerminalAt, attempt.UpdatedAt = domain.QueueAttemptCancelled, timePointer(now), now
	attempt.Version++
	return nil
}

func (service *MockCheckoutService) Start(_ context.Context, attemptID domain.AttemptID, externalUserID domain.ExternalUserID) (domain.QueueAttempt, error) {
	state := service.state
	state.mu.Lock()
	defer state.mu.Unlock()
	for _, productID := range state.productOrder {
		for _, attempt := range state.attempts[productID] {
			if attempt.ID != attemptID {
				continue
			}
			if attempt.ExternalUserID != externalUserID {
				return domain.QueueAttempt{}, domain.ErrAttemptNotFound
			}
			switch attempt.State {
			case domain.QueueAttemptCheckout:
				return cloneAttempt(*attempt), nil
			case domain.QueueAttemptInvited:
				now := time.Now().UTC()
				attempt.State, attempt.CheckoutStartedAt, attempt.CheckoutDeadline = domain.QueueAttemptCheckout, timePointer(now), timePointer(now.Add(state.checkoutTTL))
				attempt.UpdatedAt, attempt.Version = now, attempt.Version+1
				return cloneAttempt(*attempt), nil
			case domain.QueueAttemptInviteExpired, domain.QueueAttemptCheckoutExpired, domain.QueueAttemptSoldOut:
				return domain.QueueAttempt{}, domain.ErrAttemptGone
			default:
				return domain.QueueAttempt{}, domain.ErrInvalidTransition
			}
		}
	}
	return domain.QueueAttempt{}, domain.ErrAttemptNotFound
}

func (state *State) productsLocked() []domain.Product {
	products := make([]domain.Product, 0, len(state.productOrder))
	for _, id := range state.productOrder {
		product, _ := state.productLocked(id)
		products = append(products, product)
	}
	return products
}

func (state *State) productLocked(id domain.ProductID) (domain.Product, error) {
	stored, exists := state.products[id]
	if !exists {
		return domain.Product{}, domain.ErrNotFound
	}
	product := *stored
	product.WaitingCount = state.waitingCountLocked(id)
	return product, nil
}

func (state *State) activeAttemptLocked(productID domain.ProductID, userID domain.ExternalUserID) *domain.QueueAttempt {
	attempts := state.attempts[productID]
	for index := len(attempts) - 1; index >= 0; index-- {
		if attempts[index].ExternalUserID == userID && isActive(attempts[index].State) {
			return attempts[index]
		}
	}
	return nil
}

func (state *State) latestAttemptLocked(productID domain.ProductID, userID domain.ExternalUserID) *domain.QueueAttempt {
	attempts := state.attempts[productID]
	for index := len(attempts) - 1; index >= 0; index-- {
		if attempts[index].ExternalUserID == userID {
			return attempts[index]
		}
	}
	return nil
}

func (state *State) joinResultLocked(attempt domain.QueueAttempt, created bool) domain.JoinQueueResult {
	current := state.currentResultLocked(attempt)
	return domain.JoinQueueResult{Attempt: current.Attempt, PositionAhead: current.PositionAhead, TotalWaiting: current.TotalWaiting, Created: created}
}

func (state *State) currentResultLocked(attempt domain.QueueAttempt) domain.CurrentQueueResult {
	result := domain.CurrentQueueResult{Attempt: cloneAttempt(attempt), TotalWaiting: state.waitingCountLocked(attempt.ProductID)}
	if attempt.State == domain.QueueAttemptWaiting {
		for _, candidate := range state.attempts[attempt.ProductID] {
			if candidate.State == domain.QueueAttemptWaiting && candidate.QueueSequence < attempt.QueueSequence {
				result.PositionAhead++
			}
		}
	}
	return result
}

func (state *State) waitingCountLocked(productID domain.ProductID) int64 {
	var count int64
	for _, attempt := range state.attempts[productID] {
		if attempt.State == domain.QueueAttemptWaiting {
			count++
		}
	}
	return count
}

func cloneAttempt(attempt domain.QueueAttempt) domain.QueueAttempt {
	attempt.InvitedAt = cloneTime(attempt.InvitedAt)
	attempt.InvitationDeadline = cloneTime(attempt.InvitationDeadline)
	attempt.CheckoutStartedAt = cloneTime(attempt.CheckoutStartedAt)
	attempt.CheckoutDeadline = cloneTime(attempt.CheckoutDeadline)
	attempt.TerminalAt = cloneTime(attempt.TerminalAt)
	attempt.PurchasedAt = cloneTime(attempt.PurchasedAt)
	return attempt
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func isActive(state domain.QueueAttemptState) bool {
	return state == domain.QueueAttemptWaiting || state == domain.QueueAttemptInvited || state == domain.QueueAttemptCheckout
}

func reservesStock(state domain.QueueAttemptState) bool {
	return state == domain.QueueAttemptInvited || state == domain.QueueAttemptCheckout
}

func productID(raw string) domain.ProductID   { return domain.ProductID(uuid.MustParse(raw)) }
func userID(raw string) domain.ExternalUserID { return domain.ExternalUserID(raw) }
func timePointer(value time.Time) *time.Time  { return &value }
