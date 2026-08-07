package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/Woolfer0097/GoodQueue/internal/app/config"
	goodqueuehttp "github.com/Woolfer0097/GoodQueue/internal/app/http"
	"github.com/Woolfer0097/GoodQueue/internal/app/http/handler"
	"github.com/Woolfer0097/GoodQueue/internal/app/storage"
	"github.com/Woolfer0097/GoodQueue/internal/loadtest"
	"github.com/Woolfer0097/GoodQueue/internal/mockapi"
	openairecommendation "github.com/Woolfer0097/GoodQueue/internal/recommendation/openai"
	postgresrepository "github.com/Woolfer0097/GoodQueue/internal/repository/postgres"
	"github.com/Woolfer0097/GoodQueue/internal/usecase"
	"github.com/Woolfer0097/GoodQueue/internal/worker"
	"go.uber.org/zap"
)

type Application struct {
	config         config.Config
	log            *zap.Logger
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
	switch cfg.Mode {
	case config.ModeMock:
		return newMockApplication(cfg, log), nil
	case config.ModePostgres, "":
		return newPostgresApplication(cfg, log)
	default:
		return nil, fmt.Errorf("unsupported application mode %q", cfg.Mode)
	}
}

func newPostgresApplication(cfg config.Config, log *zap.Logger) (*Application, error) {
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
	var loadtestMetrics handler.LoadtestMetricsReader
	if cfg.LoadtestPrometheusURL != "" {
		loadtestMetrics = loadtest.NewPrometheusClient(cfg.LoadtestPrometheusURL, &http.Client{Timeout: 5 * time.Second})
	}
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
		LoadtestMetrics:       loadtestMetrics,
		LoadtestSuccessWindow: cfg.LoadtestSuccessWindow,
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

	return newApplication(cfg, log, router, workerSupervisor, database.Close), nil
}

func newMockApplication(cfg config.Config, log *zap.Logger) *Application {
	services := mockapi.NewServices(cfg.InvitationTTL, cfg.CheckoutTTL)
	router := goodqueuehttp.NewRouter(goodqueuehttp.Dependencies{
		Log: log, Database: alwaysReadyPinger{}, PingTimeout: cfg.DatabasePingTimeout,
		ProductService: services.Products, QueueService: services.Queue, CheckoutService: services.Checkout,
		DemoUserService: services.DemoUsers, CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	})
	log.Info("mock API enabled")
	return newApplication(cfg, log, router, noopWorker{}, func() error { return nil })
}

func newApplication(cfg config.Config, log *zap.Logger, router http.Handler, workers workerRunner, closeDatabase func() error) *Application {
	server := &http.Server{Addr: cfg.HTTPAddress, Handler: router, ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout}
	return &Application{
		config: cfg, log: log, server: server, workers: workers,
		listenAndServe: server.ListenAndServe, shutdownServer: server.Shutdown, closeDatabase: closeDatabase,
	}
}

type alwaysReadyPinger struct{}

func (alwaysReadyPinger) PingContext(context.Context) error { return nil }

type noopWorker struct{}

func (noopWorker) Run(ctx context.Context) { <-ctx.Done() }

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
