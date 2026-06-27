//go:build wireinject
// +build wireinject

package main

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	appBooking "github.com/zkw/mini-schedule/backend/internal/application/booking"
	"github.com/zkw/mini-schedule/backend/internal/application/brand"
	"github.com/zkw/mini-schedule/backend/internal/application/brandprofile"
	appClassSession "github.com/zkw/mini-schedule/backend/internal/application/classsession"
	commercialapp "github.com/zkw/mini-schedule/backend/internal/application/commercial"
	appCourseCategory "github.com/zkw/mini-schedule/backend/internal/application/coursecategory"
	appCourseTemplate "github.com/zkw/mini-schedule/backend/internal/application/coursetemplate"
	appEntitlement "github.com/zkw/mini-schedule/backend/internal/application/entitlement"
	appLearner "github.com/zkw/mini-schedule/backend/internal/application/learner"
	appLocation "github.com/zkw/mini-schedule/backend/internal/application/location"
	appLocationResource "github.com/zkw/mini-schedule/backend/internal/application/locationresource"
	appOnboarding "github.com/zkw/mini-schedule/backend/internal/application/onboarding"
	"github.com/zkw/mini-schedule/backend/internal/application/rbac"
	appRecurring "github.com/zkw/mini-schedule/backend/internal/application/recurringschedule"
	appReport "github.com/zkw/mini-schedule/backend/internal/application/report"
	appStaff "github.com/zkw/mini-schedule/backend/internal/application/staff"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	appWaitlist "github.com/zkw/mini-schedule/backend/internal/application/waitlist"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/payment"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	adminHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/admin"
	brandHandler "github.com/zkw/mini-schedule/backend/internal/interfaces/brand"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
)

// provideRBACChecker 把 Redis client 包成 cacheStore + 注入 log，给 *rbac.Checker。
// 各 service 的本地 PermissionChecker 接口在 wire.Build 里通过 wire.Bind 桥接到这一个 *rbac.Checker。
func provideRBACChecker(repo domainrbac.Repository, redisClient *redis.Client, log *slog.Logger) (*rbac.Checker, error) {
	return rbac.NewChecker(repo, rbac.NewRedisCacheStore(redisClient), log)
}

// providePublicHandler 在 brand 进程内复用 admin.Handler 的 RegisterPublicRoutes。
//
// 受 backend/.learnings deep dive 提醒，这里 brand/admin 的 NewHandler 是不同签名，
// 直接用 admin.NewHandler 需要传 brand.Service / admin user service 等不属于 brand 端语义的
// 依赖（之前 wire_gen 把它们注入了 nil）。本 provider 集中容纳这层适配，方便后续重构成
// 独立 PublicHandler。
func providePublicHandler(commercialSvc *commercialapp.Service) *adminHandler.Handler {
	return adminHandler.NewHandler(nil, commercialSvc, nil, nil, nil)
}

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
		payment.NewWeChatPaymentAdapter,

		// 仓储
		persistence.NewBrandRepository,
		persistence.NewBrandUserRepository,
		persistence.NewAppUserRepository,
		persistence.NewTrainingRepository,
		persistence.NewCommercialRepository,
		persistence.NewOnboardingRepository,
		persistence.NewLocationRepository,
		persistence.NewBrandProfileRepository,
		persistence.NewStaffRepository,
		persistence.NewRoleRepository,
		persistence.NewInstructorRepository,
		persistence.NewRBACRepository,
		// Batch 11
		persistence.NewCourseCategoryRepository,
		persistence.NewCourseTemplateRepository,
		persistence.NewClassSessionRepository,
		// Batch 12a
		persistence.NewLocationResourceRepository,
		// Batch 12b
		persistence.NewRecurringScheduleRepository,
		// Batch 13a
		persistence.NewLearnerRepository,
		// Batch 13b
		persistence.NewEntitlementRepository,
		// Batch 13c
		persistence.NewBookingRepository,
		// Batch 13d
		persistence.NewWaitlistRepository,
		// Batch 17
		persistence.NewReportRepository,

		// Batch 6/11 — RBAC checker + 各 service 的本地 PermissionChecker 接口绑定
		provideRBACChecker,
		wire.Bind(new(appStaff.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appLocation.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appOnboarding.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(brandprofile.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appCourseCategory.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appCourseTemplate.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appClassSession.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appLocationResource.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appRecurring.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appLearner.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appEntitlement.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appBooking.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appWaitlist.PermissionChecker), new(*rbac.Checker)),
		wire.Bind(new(appReport.PermissionChecker), new(*rbac.Checker)),

		// 应用服务
		brand.NewService,
		user.NewBrandUserService,
		user.NewAppUserService,
		training.NewService,
		commercialapp.NewService,
		commercialapp.NewSubscriptionGuard,
		appOnboarding.NewService,
		appLocation.NewService,
		brandprofile.NewService,
		appStaff.NewService,
		appStaff.NewRoleAllocator,
		appCourseCategory.NewService,
		appCourseTemplate.NewService,
		appClassSession.NewService,
		appLocationResource.NewService,
		appRecurring.NewService,
		appLearner.NewService,
		appEntitlement.NewService,
		appBooking.NewService,
		appWaitlist.NewService,
		appReport.NewService,

		// Handler
		brandHandler.NewHandler,
		brandHandler.NewOnboardingHandler,
		brandHandler.NewProfileHandler,
		brandHandler.NewLocationHandler,
		brandHandler.NewStaffHandler,
		brandHandler.NewMeHandler,
		brandHandler.NewCourseCategoryHandler,
		brandHandler.NewCourseTemplateHandler,
		brandHandler.NewClassSessionHandler,
		brandHandler.NewLocationResourceHandler,
		brandHandler.NewRecurringScheduleHandler,
		brandHandler.NewLearnerHandler,
		brandHandler.NewEntitlementHandler,
		brandHandler.NewBookingHandler,
		brandHandler.NewWaitlistHandler,
		brandHandler.NewReportHandler,
		providePublicHandler,

		// 路由
		newBrandRouter,
	))
}

func newBrandRouter(
	h *brandHandler.Handler,
	ph *adminHandler.Handler,
	commercialSvc *commercialapp.Service,
	roleAllocator *appStaff.RoleAllocator,
	db *gorm.DB,
	redisClient *redis.Client,
	jwtSvc *cache.Service,
	cfg *config.Config,
	log *slog.Logger,
) *gin.Engine {
	// Batch 5: 把 RoleAllocator 注入 commercial.Service，让公开注册流程
	// 在 brand_user INSERT 同事务里自动分配 brand_owner 角色。
	// 测试场景里可能两者均为 nil（router_test 仅校验 CORS），跳过。
	if commercialSvc != nil && roleAllocator != nil {
		commercialSvc.SetOwnerRoleAllocator(roleAllocator)
	}

	gin.SetMode(gin.ReleaseMode)
	if cfg.App.Debug {
		gin.SetMode(gin.DebugMode)
	}

	r := gin.New()
	r.Use(middleware.CORS(cfg.CORS))
	r.Use(middleware.Locale())
	r.Use(gin.Recovery())

	// 公开路由（注册、支付）
	publicAPI := r.Group("/api/v1/public")
	ph.RegisterPublicRoutes(publicAPI)

	// 品牌端路由
	api := r.Group("/api/v1/brand")
	h.RegisterRoutes(api)

	return r
}
