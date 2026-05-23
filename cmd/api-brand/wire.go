//go:build wireinject
// +build wireinject

package main

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/brand"
	"github.com/zkw/mini-schedule/backend/internal/application/course"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	brandHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/brand"
)

// Provider 函数：从 Config 提取子配置
func provideDatabaseConfig(cfg *config.Config) *config.DatabaseConfig {
	return &cfg.Database
}

func provideRedisConfig(cfg *config.Config) *config.RedisConfig {
	return &cfg.Redis
}

func provideJWTConfig(cfg *config.Config) *config.JWTConfig {
	return &cfg.JWT
}

// initializeBrandApp 品牌端依赖注入
func initializeBrandApp(cfg *config.Config, log *slog.Logger) (*gin.Engine, func(), error) {
	panic(wire.Build(
		// Config providers
		provideDatabaseConfig,
		provideRedisConfig,
		provideJWTConfig,

		// 基础设施
		persistence.NewDatabase,
		cache.NewRedisClient,
		cache.NewService,

		// 仓储
		persistence.NewBrandRepository,
		persistence.NewBrandUserRepository,
		persistence.NewAppUserRepository,
		persistence.NewCourseRepository,
		persistence.NewTrainingRepository,

		// 应用服务
		brand.NewService,
		user.NewBrandUserService,
		user.NewAppUserService,
		course.NewService,
		training.NewService,

		// Handler
		brandHandler.NewHandler,

		// 路由
		newBrandRouter,
	))
}

func newBrandRouter(
	h *brandHandler.Handler,
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
	r.Use(gin.Recovery())

	// 品牌端路由
	api := r.Group("/api/v1/brand")
	h.RegisterRoutes(api)

	return r
}
