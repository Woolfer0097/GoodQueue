package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const raceTestProductID = "11111111-1111-1111-1111-111111111111"

func testDatabase(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("GOODQUEUE_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://goodqueue:goodqueue@127.0.0.1:5433/goodqueue?sslmode=disable" // #nosec G101 -- local docker-compose dev credentials, not a secret
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("postgres unavailable at %s: %v", dsn, err)
	}

	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resetRaceProduct(t *testing.T, db *sql.DB) domain.ProductID {
	t.Helper()

	ctx := context.Background()
	productID, err := domain.ParseProductID(raceTestProductID)
	if err != nil {
		t.Fatalf("parse product id: %v", err)
	}

	_, err = db.ExecContext(ctx, `DELETE FROM purchase_rights WHERE product_id = $1`, uuid.UUID(productID).String())
	if err != nil {
		t.Fatalf("delete purchase rights: %v", err)
	}
	_, err = db.ExecContext(ctx, `DELETE FROM queue_entries WHERE product_id = $1`, uuid.UUID(productID).String())
	if err != nil {
		t.Fatalf("delete queue entries: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE products SET reserved = 0 WHERE id = $1`, uuid.UUID(productID).String())
	if err != nil {
		t.Fatalf("reset reserved: %v", err)
	}

	return productID
}

func TestAcquireRightRaceOnlyOneGranted(t *testing.T) {
	db := testDatabase(t)
	productID := resetRaceProduct(t, db)

	queueRepo := NewQueueRepository(db)
	rightRepo := NewPurchaseRightRepository(db)
	ctx := context.Background()

	const workers = 10
	var successCount atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := range workers {
		go func(index int) {
			defer wg.Done()
			userID := domain.ExternalUserID(uuid.NewString())
			entry, err := queueRepo.Join(ctx, productID, userID, uuid.New())
			if err != nil {
				return
			}
			_, err = rightRepo.AcquireRight(ctx, entry.TicketID, productID, 120)
			if err == nil {
				successCount.Add(1)
			} else if !errors.Is(err, domain.ErrOutOfStock) {
				t.Errorf("worker %d unexpected error: %v", index, err)
			}
		}(i)
	}

	wg.Wait()

	if got := successCount.Load(); got != 1 {
		t.Fatalf("successful acquire count = %d, want 1", got)
	}

	var reserved int
	err := db.QueryRowContext(ctx, `SELECT reserved FROM products WHERE id = $1`, uuid.UUID(productID).String()).Scan(&reserved)
	if err != nil {
		t.Fatalf("query reserved: %v", err)
	}
	if reserved != 1 {
		t.Fatalf("reserved = %d, want 1", reserved)
	}
}

func TestLeaveReleasesReservedWhenRightIssued(t *testing.T) {
	db := testDatabase(t)
	productID := resetRaceProduct(t, db)

	queueRepo := NewQueueRepository(db)
	rightRepo := NewPurchaseRightRepository(db)
	ctx := context.Background()
	userID := domain.ExternalUserID("leave-test-user")

	entry, err := queueRepo.Join(ctx, productID, userID, uuid.New())
	if err != nil {
		t.Fatalf("join queue: %v", err)
	}
	if _, err := rightRepo.AcquireRight(ctx, entry.TicketID, productID, 120); err != nil {
		t.Fatalf("acquire right: %v", err)
	}

	if err := queueRepo.Leave(ctx, productID, userID); err != nil {
		t.Fatalf("leave queue: %v", err)
	}

	var reserved int
	if err := db.QueryRowContext(ctx, `SELECT reserved FROM products WHERE id = $1`, uuid.UUID(productID).String()).Scan(&reserved); err != nil {
		t.Fatalf("query reserved: %v", err)
	}
	if reserved != 0 {
		t.Fatalf("reserved = %d, want 0 after leave", reserved)
	}

	current, err := queueRepo.Current(ctx, productID, userID)
	if err != nil {
		t.Fatalf("current entry: %v", err)
	}
	if current.Status != domain.QueueEntryCancelled {
		t.Fatalf("status = %q, want cancelled", current.Status)
	}
}

func TestListExpiredActiveRights(t *testing.T) {
	db := testDatabase(t)
	productID := resetRaceProduct(t, db)

	queueRepo := NewQueueRepository(db)
	rightRepo := NewPurchaseRightRepository(db)
	ctx := context.Background()

	entry, err := queueRepo.Join(ctx, productID, domain.ExternalUserID("expired-right-user"), uuid.New())
	if err != nil {
		t.Fatalf("join queue: %v", err)
	}
	right, err := rightRepo.AcquireRight(ctx, entry.TicketID, productID, 1)
	if err != nil {
		t.Fatalf("acquire right: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	expired, err := rightRepo.ListExpiredActiveRights(ctx, time.Now())
	if err != nil {
		t.Fatalf("list expired active rights: %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("expired rights count = %d, want 1", len(expired))
	}
	if expired[0].ID != right.ID {
		t.Fatalf("expired right id = %s, want %s", expired[0].ID, right.ID)
	}
}
