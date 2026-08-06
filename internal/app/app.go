package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	goodqueuehttp "github.com/Woolfer0097/GoodQueue/internal/app/http"
	"github.com/Woolfer0097/GoodQueue/internal/app/storage"
	"github.com/Woolfer0097/GoodQueue/internal/mockapi"
	postgresrepository "github.com/Woolfer0097/GoodQueue/internal/repository/postgres"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/Woolfer0097/GoodQueue/internal/worker"
	"go.uber.org/zap"
)

type Application struct {
	config   config.Config
	log      *zap.Logger
	database *sql.DB
	server   *http.Server
	worker   *worker.Worker
}

func New(cfg config.Config, log *zap.Logger) (*Application, error) {
	database, err := storage.OpenPostgreSQL(storage.PostgreSQLConfig{
		URL:             cfg.DatabaseURL,
		MaxOpenConns:    cfg.DatabaseMaxOpenConns,
		MaxIdleConns:    cfg.DatabaseMaxIdleConns,
		ConnMaxLifetime: cfg.DatabaseConnMaxLifetime,
	})
	if err != nil {
		return nil, err
	}

	productRepository := postgresrepository.NewProductRepository(database)
	queueRepository := postgresrepository.NewQueueRepository(database)
	purchaseRightRepository := postgresrepository.NewPurchaseRightRepository(database)
	productUseCase := usecase.NewProductUseCase(productRepository)
	queueUseCase := usecase.NewQueueUseCase(queueRepository, productRepository, purchaseRightRepository)
	dependencies := goodqueuehttp.Dependencies{
		Log:             log,
		Database:        database,
		PingTimeout:     cfg.DatabasePingTimeout,
		ProductService:  productUseCase,
		QueueService:    queueUseCase,
		CheckoutService: usecase.NewCheckoutUseCase(purchaseRightRepository),
	}
	if cfg.MockAPI {
		dependencies.ProductService = mockapi.NewProductService(productUseCase)
		dependencies.QueueService = mockapi.NewQueueService(cfg.MockQueueStatus)
		dependencies.CheckoutService = mockapi.NewCheckoutService()
		log.Info("mock API enabled", zap.String("queue_status", cfg.MockQueueStatus))
	}
	router := goodqueuehttp.NewRouter(dependencies)

	return &Application{
		config:   cfg,
		log:      log,
		database: database,
		server: &http.Server{
			Addr:              cfg.HTTPAddress,
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		},
		worker: worker.New(log, cfg.WorkerInterval, productRepository, queueUseCase, purchaseRightRepository),
	}, nil
}

func (application *Application) Run(ctx context.Context) error {
	go application.worker.Run(ctx)

	serverErrors := make(chan error, 1)
	go func() {
		application.log.Info("HTTP server listening", zap.String("address", application.server.Addr))
		serverErrors <- application.server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		_ = application.database.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), application.config.ShutdownTimeout)
		defer cancel()
		shutdownErr := application.server.Shutdown(shutdownContext)
		databaseErr := application.database.Close()
		return errors.Join(shutdownErr, databaseErr)
	}
}
