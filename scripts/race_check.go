package scripts

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"

	"github.com/Woolfer0097/GoodQueue/internal/pkg/domain"
	"github.com/Woolfer0097/GoodQueue/internal/repository/postgres"
)

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
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	db, err := sql.Open("pgx", "postgres://goodqueue:goodqueue@localhost:5432/goodqueue?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	productID, _ := domain.ParseProductID("00000000-0000-0000-0000-000000000001")

	var wg sync.WaitGroup
	results := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			repo := postgres.NewPurchaseRightRepository(db)
			// Здесь нужно создать queue_entry и получить ticket_id для реального теста.
			// Для демонстрации оставляем заглушку.
			fmt.Printf("goroutine %d trying\n", i)
		}(i)
	}
	wg.Wait()
	close(results)
	fmt.Println("done")
}