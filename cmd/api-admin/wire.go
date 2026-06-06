//go:build wireinject
// +build wireinject

package main

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/brand"
	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/payment"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	adminHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/admin"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
)

func provideDatabaseConfig(cfg *config.Config) *config.DatabaseConfig {
	return &cfg.Database
}

func provideRedisConfig(cfg *config.Config) *config.RedisConfig {
	return &cfg.Redis
}

func provideJWTConfig(cfg *config.Config) *config.JWTConfig {
	return &cfg.JWT
}

// initializeAdminApp 管理端依赖注入
func initializeAdminApp(cfg *config.Config, log *slog.Logger) (*gin.Engine, func(), error) {
	panic(wire.Build(
		provideDatabaseConfig,
		provideRedisConfig,
		provideJWTConfig,

		persistence.NewDatabase,
		cache.NewRedisClient,
		cache.NewService,
		payment.NewWeChatPaymentAdapter,

		persistence.NewBrandRepository,
		persistence.NewCommercialRepository,
		persistence.NewAdminUserRepository,

		brand.NewService,
		commercial.NewService,
		user.NewAdminUserService,

		adminHandler.NewHandler,

		newAdminRouter,
	))
}

func newAdminRouter(
	h *adminHandler.Handler,
	db *gorm.DB,
	redisClient *redis.Client,
	jwtSvc *cache.Service,
	cfg *config.Config,
	log *slog.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	if cfg.App.Debug {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(middleware.Locale())
	r.Use(gin.Recovery())

	// Swagger UI（仅 debug 模式）
	if cfg.App.Debug {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	publicAPI := r.Group("/api/v1/public")
	h.RegisterPublicRoutes(publicAPI)

	api := r.Group("/api/v1/admin")
	h.RegisterRoutes(api)

	return r
}
