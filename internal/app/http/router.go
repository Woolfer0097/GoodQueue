package http

import (
	"net/http"
	"time"

	_ "github.com/Woolfer0097/GoodQueue/internal/app/http/docs"
	"github.com/Woolfer0097/GoodQueue/internal/app/http/handler"
	"github.com/Woolfer0097/GoodQueue/internal/app/http/middleware"
	"github.com/Woolfer0097/GoodQueue/internal/app/identity"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
)

type Dependencies struct {
	Log                   *zap.Logger
	Database              handler.DatabasePinger
	PingTimeout           time.Duration
	ProductService        handler.ProductService
	QueueService          handler.QueueService
	CheckoutService       handler.CheckoutService
	DemoUserService       handler.DemoUserService
	StockService          handler.StockService
	PaymentService        handler.PaymentService
	DemoPaymentService    handler.DemoPaymentService
	UnsafeStockAdjustment bool
	UnsafePaymentCallback bool
	LoadtestMetrics       handler.LoadtestMetricsReader
	LoadtestSuccessWindow time.Duration
	AdaptiveQueueStatus   handler.AdaptiveQueueStatusReader
	CORSAllowedOrigins    []string
}

func NewRouter(dependencies Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(
		middleware.RequestIDMiddleware(),
		middleware.CORS(dependencies.CORSAllowedOrigins),
		middleware.AccessLog(dependencies.Log),
		middleware.ErrorHandler(dependencies.Log),
		middleware.Recovery(),
		identity.Optional(),
	)

	healthHandler := handler.NewHealthHandler(dependencies.Database, dependencies.PingTimeout)
	productHandler := handler.NewProductHandler(dependencies.ProductService)
	queueHandler := handler.NewQueueHandler(dependencies.QueueService)
	checkoutHandler := handler.NewCheckoutHandler(dependencies.CheckoutService)
	demoPaymentHandler := handler.NewDemoPaymentHandler(dependencies.DemoPaymentService)
	demoUserHandler := handler.NewDemoUserHandler(dependencies.DemoUserService)

	router.GET("/healthz", healthHandler.Health)
	router.GET("/readyz", healthHandler.Ready)
	router.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/index.html")
	})
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/docs/doc.json")))

	api := router.Group("/api/v1")
	api.GET("/products", productHandler.List)
	api.GET("/products/:productID", productHandler.Get)
	api.GET("/products/:productID/alternatives", productHandler.Alternatives)
	api.POST("/products/:productID/queue-entries", queueHandler.Join)
	api.GET("/products/:productID/queue-entry", queueHandler.Current)
	api.DELETE("/products/:productID/queue-entry", queueHandler.Leave)
	api.POST("/queue-attempts/:attemptID/checkout", checkoutHandler.Start)
	api.POST("/products/:productID/queue-attempts/:attemptID/demo-payment", demoPaymentHandler.Complete)
	api.GET("/demo/users", demoUserHandler.List)

	internal := router.Group("/internal/v1")
	if dependencies.AdaptiveQueueStatus != nil {
		adaptiveQueueHandler := handler.NewAdaptiveQueueHandler(dependencies.AdaptiveQueueStatus)
		internal.GET("/adaptive-queue/status", adaptiveQueueHandler.Status)
	}
	if dependencies.LoadtestMetrics != nil {
		loadtestMetricsHandler := handler.NewLoadtestMetricsHandler(dependencies.LoadtestMetrics, dependencies.LoadtestSuccessWindow)
		internal.GET("/loadtest/request-success-rate", loadtestMetricsHandler.RequestSuccessRate)
		internal.GET("/loadtest/purchase-success-rate", loadtestMetricsHandler.PurchaseSuccessRate)
	}
	if dependencies.UnsafeStockAdjustment && dependencies.StockService != nil {
		stockHandler := handler.NewStockHandler(dependencies.StockService)
		dependencies.Log.Warn("UNSAFE unauthenticated stock adjustment enabled; demo use only",
			zap.String("path", "/internal/v1/products/:productID/stock-adjustments"))
		internal.POST("/products/:productID/stock-adjustments", stockHandler.Adjust)
	}
	if dependencies.UnsafePaymentCallback && dependencies.PaymentService != nil {
		paymentHandler := handler.NewPaymentHandler(dependencies.PaymentService)
		dependencies.Log.Warn("UNSAFE unauthenticated payment callback enabled; demo use only",
			zap.String("path", "/internal/v1/payment-events"))
		internal.POST("/payment-events", paymentHandler.Process)
	}

	return router
}
