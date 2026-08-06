package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	goodqueuehttp "github.com/Woolfer0097/GoodQueue/internal/app/http"
	"github.com/Woolfer0097/GoodQueue/internal/app/storage"
	openairecommendation "github.com/Woolfer0097/GoodQueue/internal/recommendation/openai"
	postgresrepository "github.com/Woolfer0097/GoodQueue/internal/repository/postgres"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/Woolfer0097/GoodQueue/internal/worker"
	"go.uber.org/zap"
)

type Application struct {
	config         config.Config
	log            *zap.Logger
	database       *sql.DB
	server         *http.Server
	workers        workerRunner
	listenAndServe func() error
	shutdownServer func(context.Context) error
	closeDatabase  func() error
	runMu          sync.Mutex
	started        bool
}

type workerRunner interface {
	Run(context.Context)
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

	productRepository := postgresrepository.NewProductRepository(database, cfg.WaitingBufferPercent)
	recommendationRepository := postgresrepository.NewRecommendationRepository(database, cfg.WaitingBufferPercent)
	var embeddingProvider usecase.EmbeddingProvider
	if cfg.RecommendationsAIEnabled {
		embeddingProvider, err = openairecommendation.NewEmbedder(
			cfg.OpenAIAPIKey,
			cfg.OpenAIEmbeddingModel,
			cfg.OpenAIBaseURL,
			&http.Client{Timeout: cfg.OpenAIEmbeddingTimeout},
		)
		if err != nil {
			_ = database.Close()
			return nil, fmt.Errorf("configure AI recommendations: %w", err)
		}
	}
	queueAttemptRepository := postgresrepository.NewQueueAttemptRepository(
		database,
		cfg.InvitationTTL,
		cfg.CheckoutTTL,
		cfg.WaitingBufferPercent,
	)
	queueUseCase := usecase.NewQueueUseCase(queueAttemptRepository)
	paymentUseCase := usecase.NewPaymentUseCase(queueAttemptRepository)
	router := goodqueuehttp.NewRouter(goodqueuehttp.Dependencies{
		Log:         log,
		Database:    database,
		PingTimeout: cfg.DatabasePingTimeout,
		ProductService: usecase.NewProductUseCase(
			productRepository,
			recommendationRepository,
			embeddingProvider,
		),
		QueueService:          queueUseCase,
		CheckoutService:       usecase.NewCheckoutUseCase(queueAttemptRepository),
		DemoUserService:       usecase.NewDemoUserUseCase(postgresrepository.NewDemoUserRepository(database)),
		StockService:          usecase.NewStockUseCase(queueAttemptRepository),
		PaymentService:        paymentUseCase,
		UnsafePaymentCallback: cfg.UnsafePaymentCallback,
		CORSAllowedOrigins:    cfg.CORSAllowedOrigins,
	})
	outboxRepository := postgresrepository.NewNotificationOutboxRepository(database)
	workerSupervisor := worker.NewSupervisor(worker.Config{
		Interval:                cfg.WorkerInterval,
		ReconciliationBatchSize: cfg.ReconciliationBatchSize,
		MaxReconciledProducts:   cfg.MaxProductsPerCycle,
		MaxOutboxItems:          cfg.MaxOutboxItemsPerCycle,
		OutboxLeaseDuration:     cfg.OutboxLeaseDuration,
		OutboxRetryBase:         cfg.OutboxRetryBase,
		OutboxRetryMax:          cfg.OutboxRetryMax,
		PublisherTimeout:        cfg.PublisherTimeout,
	}, queueAttemptRepository, outboxRepository, worker.NewLoggingPublisher(log), worker.NoopObserver{}, log)

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           router,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
	}
	return &Application{
		config:         cfg,
		log:            log,
		database:       database,
		server:         server,
		workers:        workerSupervisor,
		listenAndServe: server.ListenAndServe,
		shutdownServer: server.Shutdown,
		closeDatabase:  database.Close,
	}, nil
}

func (application *Application) Run(ctx context.Context) error {
	application.runMu.Lock()
	if application.started {
		application.runMu.Unlock()
		return fmt.Errorf("application may only be run once")
	}
	application.started = true
	application.runMu.Unlock()

	workerContext, cancelWorkers := context.WithCancel(context.Background())
	workersDone := make(chan struct{})
	go func() {
		defer close(workersDone)
		application.workers.Run(workerContext)
	}()

	serverErrors := make(chan error, 1)
	go func() {
		application.log.Info("HTTP server listening", zap.String("address", application.server.Addr))
		serverErrors <- application.listenAndServe()
	}()

	var runErr error
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = fmt.Errorf("serve HTTP: %w", err)
		}
	case <-ctx.Done():
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), application.config.ShutdownTimeout)
	defer cancelShutdown()
	cancelWorkers()
	shutdownErr := application.shutdownServer(shutdownContext)
	workersErr := waitForWorkers(shutdownContext, workersDone)
	if workersErr != nil {
		return errors.Join(runErr, shutdownErr, workersErr)
	}
	databaseErr := application.closeDatabase()
	return errors.Join(runErr, shutdownErr, workersErr, databaseErr)
}

func waitForWorkers(ctx context.Context, workersDone <-chan struct{}) error {
	select {
	case <-workersDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for workers: %w", ctx.Err())
	}
}
