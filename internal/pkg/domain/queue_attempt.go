package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const maxIdempotencyKeyLength = 128

const (
	MaxStockAdjustmentReasonLength            = 200
	MaxStockAdjustmentExternalReferenceLength = 200
)

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

type AttemptID uuid.UUID

func ParseAttemptID(raw string) (AttemptID, error) {
	value, err := uuid.Parse(raw)
	if err != nil {
		return AttemptID{}, fmt.Errorf("%w: attempt ID must be a UUID", ErrInvalidInput)
	}
	return AttemptID(value), nil
}

type IdempotencyKey string

func ParseIdempotencyKey(raw string) (IdempotencyKey, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) > maxIdempotencyKeyLength || !idempotencyKeyPattern.MatchString(trimmed) {
		return "", fmt.Errorf(
			"%w: idempotency key must contain 1..%d ASCII letters, digits, dots, underscores, colons, or hyphens",
			ErrInvalidInput,
			maxIdempotencyKeyLength,
		)
	}
	return IdempotencyKey(trimmed), nil
}

type QueueAttemptState string

const (
	QueueAttemptWaiting         QueueAttemptState = "waiting"
	QueueAttemptInvited         QueueAttemptState = "invited"
	QueueAttemptCheckout        QueueAttemptState = "checkout"
	QueueAttemptPurchased       QueueAttemptState = "purchased"
	QueueAttemptInviteExpired   QueueAttemptState = "invite_expired"
	QueueAttemptCheckoutExpired QueueAttemptState = "checkout_expired"
	QueueAttemptPaymentFailed   QueueAttemptState = "payment_failed"
	QueueAttemptCancelled       QueueAttemptState = "cancelled"
	QueueAttemptSoldOut         QueueAttemptState = "sold_out"
)

type QueueAttempt struct {
	ID                 AttemptID
	ProductID          ProductID
	QueueSequence      int64
	ExternalUserID     ExternalUserID
	IdempotencyKey     IdempotencyKey
	State              QueueAttemptState
	CreatedAt          time.Time
	UpdatedAt          time.Time
	InvitedAt          *time.Time
	InvitationDeadline *time.Time
	CheckoutStartedAt  *time.Time
	CheckoutDeadline   *time.Time
	TerminalAt         *time.Time
	PurchasedAt        *time.Time
	AcceptedProvider   *string
	AcceptedReference  *string
	TerminalReason     *string
	TerminalMessage    *string
	Version            int64
}

type JoinQueueCommand struct {
	ProductID      ProductID
	ExternalUserID ExternalUserID
	IdempotencyKey IdempotencyKey
}

type JoinQueueResult struct {
	Attempt       QueueAttempt
	PositionAhead int64
	TotalWaiting  int64
	Created       bool
}

func ParseStockAdjustmentCommand(
	productID ProductID,
	idempotencyKey IdempotencyKey,
	delta int64,
	reason, externalReference string,
) (StockAdjustmentCommand, error) {
	if delta == 0 || delta < math.MinInt32 || delta > math.MaxInt32 {
		return StockAdjustmentCommand{}, fmt.Errorf("%w: stock delta must be a non-zero 32-bit integer", ErrInvalidInput)
	}
	normalizedReason := strings.TrimSpace(reason)
	if normalizedReason == "" || len(normalizedReason) > MaxStockAdjustmentReasonLength {
		return StockAdjustmentCommand{}, fmt.Errorf("%w: reason must contain 1..%d bytes", ErrInvalidInput, MaxStockAdjustmentReasonLength)
	}
	normalizedReference := strings.TrimSpace(externalReference)
	if normalizedReference == "" || len(normalizedReference) > MaxStockAdjustmentExternalReferenceLength {
		return StockAdjustmentCommand{}, fmt.Errorf("%w: external reference must contain 1..%d bytes", ErrInvalidInput, MaxStockAdjustmentExternalReferenceLength)
	}
	return StockAdjustmentCommand{
		ProductID: productID, IdempotencyKey: idempotencyKey, Delta: int32(delta),
		Reason: normalizedReason, ExternalReference: normalizedReference,
	}, nil
}

type StartCheckoutCommand struct {
	AttemptID      AttemptID
	ExternalUserID ExternalUserID
}

type CancelQueueCommand struct {
	ProductID      ProductID
	ExternalUserID ExternalUserID
}

type CurrentQueueResult struct {
	Attempt       QueueAttempt
	PositionAhead int64
	TotalWaiting  int64
}

type StockAdjustmentCommand struct {
	ProductID         ProductID
	IdempotencyKey    IdempotencyKey
	Delta             int32
	Reason            string
	ExternalReference string
}

type StockAdjustmentResult struct {
	AdjustmentID uuid.UUID
	StockBefore  int32
	StockAfter   int32
	HTTPStatus   int
	ResponseBody []byte
	Replayed     bool
}

func WaitingCapacity(allocatableStock int32, bufferPercent int) (int64, error) {
	if allocatableStock < 0 || bufferPercent < 0 {
		return 0, fmt.Errorf("%w: stock and buffer percent must be non-negative", ErrInvalidInput)
	}
	stock := int64(allocatableStock)
	percent := int64(bufferPercent)
	if stock != 0 && percent > math.MaxInt64/stock {
		return 0, fmt.Errorf("%w: waiting capacity overflow", ErrInvalidInput)
	}
	product := stock * percent
	if product > math.MaxInt64-99 {
		return 0, fmt.Errorf("%w: waiting capacity overflow", ErrInvalidInput)
	}
	return (product + 99) / 100, nil
}

func CanonicalStockAdjustmentHash(command StockAdjustmentCommand) [sha256.Size]byte {
	fields := []string{
		"v1",
		strings.ToLower(uuid.UUID(command.ProductID).String()),
		strconv.FormatInt(int64(command.Delta), 10),
		strings.TrimSpace(command.Reason),
		strings.TrimSpace(command.ExternalReference),
	}
	hasher := sha256.New()
	var length [4]byte
	for _, field := range fields {
		binary.BigEndian.PutUint32(length[:], canonicalFieldLength(field))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(field))
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}

func canonicalFieldLength(field string) uint32 {
	if uint64(len(field)) > math.MaxUint32 {
		panic("canonical stock adjustment field exceeds uint32 length")
	}
	return uint32(len(field)) //nolint:gosec // The explicit upper-bound guard makes the conversion lossless.
}

func NextQueueSequence(current int64) (int64, error) {
	if current < 1 {
		return 0, fmt.Errorf("%w: queue sequence must be positive", ErrInvalidInput)
	}
	if current == math.MaxInt64 {
		return 0, ErrQueueSequenceExhausted
	}
	return current + 1, nil
}
