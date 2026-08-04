//go:build ignore

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/repository/postgres"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://goodqueue:goodqueue@localhost:5433/goodqueue?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	productID, _ := domain.ParseProductID("11111111-1111-1111-1111-111111111111")

	// Очистка в правильном порядке (сначала purchase_rights, потом queue_entries)
	_, err = db.ExecContext(ctx, `
		DELETE FROM purchase_rights WHERE product_id = $1
	`, uuid.UUID(productID).String())
	if err != nil {
		log.Printf("Ошибка удаления прав: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		DELETE FROM queue_entries WHERE product_id = $1
	`, uuid.UUID(productID).String())
	if err != nil {
		log.Printf("Ошибка удаления записей очереди: %v", err)
	}

	_, err = db.ExecContext(ctx, `
		UPDATE products SET reserved = 0 WHERE id = $1
	`, uuid.UUID(productID).String())
	if err != nil {
		log.Printf("Ошибка сброса reserved: %v", err)
	}
	fmt.Println("🧹 Очистка выполнена")

	userIDs := []domain.ExternalUserID{"user1", "user2", "user3", "user4", "user5", "user6", "user7", "user8", "user9", "user10"}

	var wg sync.WaitGroup
	type result struct {
		user domain.ExternalUserID
		ok   bool
		msg  string
		err  error
	}
	results := make(chan result, len(userIDs))

	queueRepo := postgres.NewQueueRepository(db)
	rightRepo := postgres.NewPurchaseRightRepository(db)

	for _, uid := range userIDs {
		wg.Add(1)
		go func(userID domain.ExternalUserID) {
			defer wg.Done()
			// Встаём в очередь
			idempotencyKey := uuid.New()
			entry, err := queueRepo.Join(ctx, productID, userID, idempotencyKey)
			if err != nil {
				results <- result{user: userID, ok: false, msg: "join error", err: err}
				return
			}
			// Пытаемся получить право
			right, err := rightRepo.AcquireRight(ctx, entry.TicketID, productID, 120)
			if err != nil {
				results <- result{user: userID, ok: false, msg: "acquire error", err: err}
				return
			}
			results <- result{user: userID, ok: true, msg: fmt.Sprintf("right %s", right.ID)}
		}(uid)
	}

	wg.Wait()
	close(results)

	successCount := 0
	var errors []string
	for res := range results {
		if res.ok {
			successCount++
			fmt.Printf("✅ %s: %s\n", res.user, res.msg)
		} else {
			errMsg := fmt.Sprintf("%s: %s - %v", res.user, res.msg, res.err)
			errors = append(errors, errMsg)
		}
	}
	fmt.Printf("\n✅ Успешно получено прав: %d (должно быть 1)\n", successCount)
	if len(errors) > 0 {
		fmt.Println("❌ Ошибки у остальных:")
		for _, e := range errors {
			fmt.Println(" -", e)
		}
	}

	// Проверяем reserved после теста
	var reserved int
	err = db.QueryRowContext(ctx, `SELECT reserved FROM products WHERE id = $1`, uuid.UUID(productID).String()).Scan(&reserved)
	if err == nil {
		fmt.Printf("📊 reserved = %d (должно быть 1)\n", reserved)
	}
}
