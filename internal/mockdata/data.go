package mockdata

import (
	"fmt"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

const (
	QueueEntryID = int64(42)

	QueueStatusWaiting   = "waiting"
	QueueStatusGranted   = "granted"
	QueueStatusPurchased = "purchased"
	QueueStatusCancelled = "cancelled"
	QueueStatusExpired   = "expired"
)

var (
	consoleID       = uuid.MustParse("280f1230-81e3-4e10-aad6-864d8bb12a78")
	graphicsCardID  = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	keyboardID      = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	authorizationID = uuid.MustParse("41cd68a0-5e63-4d6e-a610-b5d3281a4fea")
	joinedAt        = time.Date(2026, time.August, 4, 10, 0, 0, 0, time.UTC)
	authorizedAt    = time.Date(2026, time.August, 4, 10, 16, 20, 0, time.UTC)
	expiresAt       = time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)

	mockProducts = []domain.Product{
		{
			ID:               domain.ProductID(consoleID),
			Title:            "Лимитированная игровая приставка",
			Description:      "Игровая приставка ограниченной серии с двумя беспроводными контроллерами.",
			ImageURL:         "https://placehold.co/1200x800/png?text=Limited+Console",
			PriceKopecks:     1999900,
			AllocatableStock: 1,
			QueueEnabled:     true,
			RightTTLSeconds:  120,
		},
		{
			ID:               domain.ProductID(graphicsCardID),
			Title:            "Видеокарта RTX 5070",
			Description:      "Игровая видеокарта с 12 ГБ видеопамяти.",
			ImageURL:         "https://placehold.co/1200x800/png?text=RTX+5070",
			PriceKopecks:     8999900,
			AllocatableStock: 4,
			QueueEnabled:     true,
			RightTTLSeconds:  180,
		},
		{
			ID:               domain.ProductID(keyboardID),
			Title:            "Механическая клавиатура",
			Description:      "Компактная механическая клавиатура.",
			ImageURL:         "https://placehold.co/1200x800/png?text=Keyboard",
			PriceKopecks:     1699900,
			AllocatableStock: 12,
			QueueEnabled:     false,
			RightTTLSeconds:  600,
		},
	}
)

func Products() []domain.Product {
	products := make([]domain.Product, len(mockProducts))
	copy(products, mockProducts)
	return products
}

func HasProduct(productID domain.ProductID) bool {
	for _, product := range mockProducts {
		if product.ID == productID {
			return true
		}
	}
	return false
}

func QueueEntry(status string, productID domain.ProductID, userID domain.ExternalUserID) (domain.QueueEntry, error) {
	entry := domain.QueueEntry{
		TicketID:       QueueEntryID,
		ProductID:      productID,
		ExternalUserID: userID,
		JoinedAt:       joinedAt,
	}

	switch status {
	case QueueStatusWaiting:
		position, totalWaiting := 3, 7
		entry.Status = domain.QueueEntryWaiting
		entry.Position = &position
		entry.TotalWaiting = &totalWaiting
	case QueueStatusGranted:
		rightIssuedAt, expiry := authorizedAt, expiresAt
		entry.Status = domain.QueueEntryRightIssued
		entry.RightIssuedAt = &rightIssuedAt
		entry.ExpiredAt = &expiry
	case QueueStatusPurchased:
		rightIssuedAt, completedAt := authorizedAt, authorizedAt
		entry.Status = domain.QueueEntryCompleted
		entry.RightIssuedAt = &rightIssuedAt
		entry.CompletedAt = &completedAt
	case QueueStatusCancelled:
		cancelledAt := authorizedAt
		entry.Status = domain.QueueEntryCancelled
		entry.CancelledAt = &cancelledAt
	case QueueStatusExpired:
		rightIssuedAt, expiry := authorizedAt, expiresAt
		entry.Status = domain.QueueEntryExpired
		entry.RightIssuedAt = &rightIssuedAt
		entry.ExpiredAt = &expiry
	default:
		return domain.QueueEntry{}, fmt.Errorf("unsupported mock queue status %q", status)
	}

	return entry, nil
}

func CheckoutAuthorization() domain.PurchaseRight {
	consumedAt := authorizedAt
	return domain.PurchaseRight{
		ID:            authorizationID,
		QueueTicketID: QueueEntryID,
		Status:        domain.PurchaseRightConsumed,
		IssuedAt:      authorizedAt,
		ExpiresAt:     expiresAt,
		ConsumedAt:    &consumedAt,
	}
}
