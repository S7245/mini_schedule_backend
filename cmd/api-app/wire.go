//go:build wireinject
// +build wireinject

package main

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/application/course"
	"github.com/zkw/mini-schedule/backend/internal/application/learnerbooking"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	appHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/app"
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

// initializeAppApp C 端依赖注入
func initializeAppApp(cfg *config.Config, log *slog.Logger) (*gin.Engine, func(), error) {
	panic(wire.Build(
		provideDatabaseConfig,
		provideRedisConfig,
		provideJWTConfig,

		persistence.NewDatabase,
		cache.NewRedisClient,
		cache.NewService,

		persistence.NewAppUserRepository,
		persistence.NewCourseRepository,
		persistence.NewTrainingRepository,
		persistence.NewBookingRepository,
		persistence.NewClassSessionRepository,
		persistence.NewLearnerRepository,
		persistence.NewEntitlementRepository,
		persistence.NewWaitlistRepository,

		commercial.NewSubscriptionGuard,

		user.NewAppUserService,
		course.NewService,
		training.NewService,
		learnerbooking.NewService,

		appHandler.NewHandler,

		newAppRouter,
	))
}

func newAppRouter(
	h *appHandler.Handler,
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
	r.Use(middleware.Locale())
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(gin.Recovery())

	api := r.Group("/api/v1/app")
	h.RegisterRoutes(api)

	return r
}
