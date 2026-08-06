package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
)

const maxOutboxErrorLength = 500

type NotificationOutboxRepository struct {
	db               *sql.DB
	afterClaimLocked func(domain.NotificationClaim)
	afterFinalized   func(uuid.UUID)
}

func NewNotificationOutboxRepository(db *sql.DB) *NotificationOutboxRepository {
	return &NotificationOutboxRepository{db: db}
}

func (repository *NotificationOutboxRepository) ClaimNotification(
	ctx context.Context,
	leaseDuration time.Duration,
) (*domain.NotificationClaim, error) {
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("%w: outbox lease duration must be positive", domain.ErrInvalidInput)
	}
	tx, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var now time.Time
	if err := tx.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, fmt.Errorf("read outbox claim clock: %w", err)
	}
	row := tx.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id FROM notification_outbox
			WHERE ((status IN ('pending','failed') AND available_at <= $1)
				OR (status='processing' AND lease_until <= $1))
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE notification_outbox o
		SET status='processing', lease_token=$2, lease_generation=o.lease_generation+1,
			lease_until=$3, attempt_count=o.attempt_count+1, updated_at=$1, last_error=NULL
		FROM candidate c WHERE o.id=c.id
		RETURNING o.id,o.attempt_id,o.event_type,o.deduplication_key,o.payload,o.payload_hash,
			o.attempt_count,o.lease_token,o.lease_generation,o.lease_until`, now, uuid.New(), now.Add(leaseDuration))
	claim, err := scanNotificationClaim(row)
	if errors.Is(err, sql.ErrNoRows) {
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, fmt.Errorf("commit empty outbox claim: %w", commitErr)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if repository.afterClaimLocked != nil {
		repository.afterClaimLocked(claim)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit outbox claim: %w", err)
	}
	return &claim, nil
}

func scanNotificationClaim(scanner rowScanner) (domain.NotificationClaim, error) {
	var claim domain.NotificationClaim
	var attemptID *uuid.UUID
	var payloadHash []byte
	err := scanner.Scan(&claim.ID, &attemptID, &claim.EventType, &claim.DeduplicationKey, &claim.Payload,
		&payloadHash, &claim.AttemptCount, &claim.LeaseToken, &claim.LeaseGeneration, &claim.LeaseUntil)
	if err != nil {
		return domain.NotificationClaim{}, err
	}
	if len(payloadHash) != sha256.Size {
		return domain.NotificationClaim{}, fmt.Errorf("%w: outbox payload hash has %d bytes", domain.ErrInternal, len(payloadHash))
	}
	copy(claim.PayloadHash[:], payloadHash)
	if attemptID != nil {
		value := domain.AttemptID(*attemptID)
		claim.AttemptID = &value
	}
	return claim, nil
}

func (repository *NotificationOutboxRepository) ClassifyInvitation(
	ctx context.Context,
	claim domain.NotificationClaim,
) (bool, error) {
	if claim.EventType != invitationEventType {
		return false, nil
	}
	if claim.AttemptID == nil {
		return true, nil
	}
	var payload struct {
		Deadline time.Time `json:"invitation_deadline"`
	}
	if err := json.Unmarshal(claim.Payload, &payload); err != nil || payload.Deadline.IsZero() {
		return true, nil
	}
	var current bool
	err := repository.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM queue_attempts
			WHERE id=$1 AND state='invited' AND invitation_deadline=$2
				AND clock_timestamp() < invitation_deadline
		)`, uuid.UUID(*claim.AttemptID), payload.Deadline).Scan(&current)
	if err != nil {
		return false, fmt.Errorf("classify invitation notification: %w", err)
	}
	return !current, nil
}

func (repository *NotificationOutboxRepository) MarkNotificationSent(ctx context.Context, claim domain.NotificationClaim) error {
	return repository.finalizeNotification(ctx, claim, nil)
}

func (repository *NotificationOutboxRepository) MarkNotificationObsolete(ctx context.Context, claim domain.NotificationClaim) error {
	reason := "obsolete"
	return repository.finalizeNotification(ctx, claim, &reason)
}

func (repository *NotificationOutboxRepository) finalizeNotification(
	ctx context.Context,
	claim domain.NotificationClaim,
	lastError *string,
) error {
	result, err := repository.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status='sent',sent_at=clock_timestamp(),updated_at=clock_timestamp(),
			lease_until=NULL,lease_token=NULL,last_error=$1
		WHERE id=$2 AND status='processing' AND lease_token=$3 AND lease_generation=$4 AND payload_hash=$5`,
		lastError, claim.ID, claim.LeaseToken, claim.LeaseGeneration, claim.PayloadHash[:])
	if err != nil {
		return fmt.Errorf("finalize outbox notification: %w", err)
	}
	if err := staleUnlessOneRow(result); err != nil {
		return err
	}
	if repository.afterFinalized != nil {
		repository.afterFinalized(claim.ID)
	}
	return nil
}

func (repository *NotificationOutboxRepository) RetryNotification(
	ctx context.Context,
	claim domain.NotificationClaim,
	publicationError error,
	baseBackoff time.Duration,
	maxBackoff time.Duration,
) (time.Duration, error) {
	if baseBackoff <= 0 || maxBackoff < baseBackoff {
		return 0, fmt.Errorf("%w: invalid outbox retry backoff", domain.ErrInvalidInput)
	}
	backoff := exponentialBackoff(claim.AttemptCount, baseBackoff, maxBackoff)
	boundedError := publicationError.Error()
	if len(boundedError) > maxOutboxErrorLength {
		boundedError = boundedError[:maxOutboxErrorLength]
	}
	result, err := repository.db.ExecContext(ctx, `
		UPDATE notification_outbox
		SET status='failed',available_at=clock_timestamp()+$1,updated_at=clock_timestamp(),
			lease_until=NULL,lease_token=NULL,last_error=$2
		WHERE id=$3 AND status='processing' AND lease_token=$4 AND lease_generation=$5 AND payload_hash=$6`,
		backoff, boundedError, claim.ID, claim.LeaseToken, claim.LeaseGeneration, claim.PayloadHash[:])
	if err != nil {
		return 0, fmt.Errorf("schedule outbox retry: %w", err)
	}
	if err := staleUnlessOneRow(result); err != nil {
		return 0, err
	}
	return backoff, nil
}

func exponentialBackoff(attemptCount int, baseBackoff, maxBackoff time.Duration) time.Duration {
	if attemptCount <= 1 {
		return baseBackoff
	}
	exponent := attemptCount - 1
	if exponent >= 63 || baseBackoff > maxBackoff/time.Duration(int64(1)<<min(exponent, 62)) {
		return maxBackoff
	}
	backoff := baseBackoff * time.Duration(int64(1)<<exponent)
	return min(backoff, maxBackoff)
}

func staleUnlessOneRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read outbox update count: %w", err)
	}
	if affected == 0 {
		return domain.ErrStaleClaim
	}
	if affected != 1 {
		return fmt.Errorf("%w: outbox update affected %d rows", domain.ErrInternal, affected)
	}
	return nil
}
