package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gabrielcamargo/oficina-api/configs"
	_ "github.com/gabrielcamargo/oficina-api/docs"
	"github.com/gabrielcamargo/oficina-api/internal/api/handler"
	"github.com/gabrielcamargo/oficina-api/internal/api/middleware"
	"github.com/gabrielcamargo/oficina-api/internal/application/saga"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/auth"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/customer"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/serviceorder"
	"github.com/gabrielcamargo/oficina-api/internal/application/usecase/vehicle"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/external"
	"github.com/gabrielcamargo/oficina-api/internal/infrastructure/notification"
	infrarepo "github.com/gabrielcamargo/oficina-api/internal/infrastructure/repository"
	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"gorm.io/gorm"
)

type Server struct {
	router       *gin.Engine
	config       *configs.Config
	db           *gorm.DB
	orchestrator *saga.Orchestrator
	httpServer   *http.Server
}

func NewServer(cfg *configs.Config, db *gorm.DB) *Server {
	router := gin.New()
	router.Use(
		gin.CustomRecovery(recoverWithLog),
		otelgin.Middleware(cfg.ServiceName),
		middleware.RequestID(),
		middleware.RequestLogger(),
	)

	server := &Server{
		router: router,
		config: cfg,
		db:     db,
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", cfg.ServerPort),
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
		},
	}

	server.registerRoutes()

	return server
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) Run() error {
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func recoverWithLog(c *gin.Context, recovered any) {
	slog.ErrorContext(c.Request.Context(), "http.panic",
		slog.String("event", "http.panic"),
		slog.Any("panic", recovered),
	)
	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func (s *Server) SagaOrchestrator() *saga.Orchestrator {
	return s.orchestrator
}

func (s *Server) registerRoutes() {
	userRepo := infrarepo.NewGormUserRepository(s.db)
	customerRepo := infrarepo.NewGormCustomerRepository(s.db)
	vehicleRepo := infrarepo.NewGormVehicleRepository(s.db)
	orderRepo := infrarepo.NewGormServiceOrderRepository(s.db)
	sagaRepo := infrarepo.NewGormSagaRepository(s.db)
	cnpjSvc := external.NewBrasilAPIClient()
	emailNotifier := notification.NewResendNotifier(s.config.ResendAPIKey, s.config.EmailFrom)
	statusNotifier := serviceorder.NewOrderStatusNotifier(customerRepo, emailNotifier)

	s.orchestrator = saga.NewOrchestrator(sagaRepo, orderRepo, customerRepo, statusNotifier, s.config.SagaStepTimeout)

	authHandler := handler.NewAuthHandler(
		auth.NewCreateUserUseCase(userRepo),
		auth.NewLoginUseCase(userRepo, s.config.JWTSecret),
	)

	customerHandler := handler.NewCustomerHandler(
		customer.NewCreateCustomerUseCase(customerRepo, cnpjSvc),
		customer.NewUpdateCustomerUseCase(customerRepo),
		customer.NewChangeCustomerStatusUseCase(customerRepo),
		customer.NewDeleteCustomerUseCase(customerRepo),
		customer.NewGetCustomerUseCase(customerRepo),
		customer.NewListCustomersUseCase(customerRepo),
	)

	internalCustomerHandler := handler.NewInternalCustomerHandler(
		customer.NewFindCustomerByDocumentUseCase(customerRepo),
	)

	vehicleHandler := handler.NewVehicleHandler(
		vehicle.NewCreateVehicleUseCase(vehicleRepo, customerRepo),
		vehicle.NewUpdateVehicleUseCase(vehicleRepo),
		vehicle.NewDeleteVehicleUseCase(vehicleRepo),
		vehicle.NewGetVehicleUseCase(vehicleRepo),
		vehicle.NewListVehiclesUseCase(vehicleRepo),
	)

	orderHandler := handler.NewServiceOrderHandler(
		serviceorder.NewCreateServiceOrderUseCase(customerRepo, vehicleRepo, s.orchestrator),
		serviceorder.NewCancelServiceOrderUseCase(s.orchestrator),
		serviceorder.NewDeliverVehicleUseCase(orderRepo, statusNotifier),
		serviceorder.NewGetServiceOrderUseCase(orderRepo),
		serviceorder.NewListServiceOrdersUseCase(orderRepo),
		serviceorder.NewGetServiceOrderHistoryUseCase(orderRepo),
		serviceorder.NewGetServiceOrderSagaUseCase(orderRepo, sagaRepo),
	)

	s.router.GET("/health", s.health)
	s.router.GET("/ready", s.ready)
	s.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	internal := s.router.Group("/internal", middleware.InternalAuth(s.config.InternalAPISecret))
	internalCustomerHandler.RegisterRoutes(internal)

	v1 := s.router.Group("/api/v1")
	authenticated := v1.Group("", middleware.Auth(s.config.JWTSecret))
	operators := authenticated.Group("", middleware.RequireRole(middleware.RoleOperator))

	authHandler.RegisterRoutes(v1)
	orderHandler.RegisterPublicRoutes(v1)
	orderHandler.RegisterSharedRoutes(authenticated)
	orderHandler.RegisterOperatorRoutes(operators)
	customerHandler.RegisterRoutes(operators)
	vehicleHandler.RegisterRoutes(operators)
}

func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) ready(c *gin.Context) {
	sqlDB, err := s.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
		return
	}
	if err := sqlDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "database unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
