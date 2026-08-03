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
	Log             *zap.Logger
	Database        handler.DatabasePinger
	PingTimeout     time.Duration
	ProductService  handler.ProductService
	QueueService    handler.QueueService
	CheckoutService handler.CheckoutService
}

func NewRouter(dependencies Dependencies) *gin.Engine {
	router := gin.New()
	router.Use(
		middleware.RequestIDMiddleware(),
		middleware.AccessLog(dependencies.Log),
		middleware.ErrorHandler(dependencies.Log),
		middleware.Recovery(),
		identity.Optional(),
	)

	healthHandler := handler.NewHealthHandler(dependencies.Database, dependencies.PingTimeout)
	productHandler := handler.NewProductHandler(dependencies.ProductService)
	queueHandler := handler.NewQueueHandler(dependencies.QueueService)
	checkoutHandler := handler.NewCheckoutHandler(dependencies.CheckoutService)

	router.GET("/healthz", healthHandler.Health)
	router.GET("/readyz", healthHandler.Ready)
	router.GET("/docs", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/docs/index.html")
	})
	router.GET("/docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/docs/doc.json")))

	api := router.Group("/api/v1")
	api.GET("/products", productHandler.List)
	api.GET("/products/:productID", productHandler.Get)
	api.POST("/products/:productID/queue-entries", queueHandler.Join)
	api.GET("/products/:productID/queue-entry", queueHandler.Current)
	api.DELETE("/products/:productID/queue-entry", queueHandler.Leave)
	api.POST("/products/:productID/checkout-authorizations", checkoutHandler.Authorize)

	return router
}
