package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	brandapp "github.com/zkw/mini-schedule/backend/internal/application/brand"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	trainingdomain "github.com/zkw/mini-schedule/backend/internal/domain/training"
	domainuser "github.com/zkw/mini-schedule/backend/internal/domain/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
	"github.com/zkw/mini-schedule/backend/pkg/validation"
)

// Handler 品牌商家后台 Handler
type Handler struct {
	brandSvc     *brandapp.Service
	brandUserSvc *user.BrandUserService
	appUserSvc   *user.AppUserService
	trainingSvc  *training.Service
	jwtSvc       *cache.Service
	validator    *validation.Validator

	// Batch 4 — 子 handler，按域拆分
	onboarding *OnboardingHandler
	profile    *ProfileHandler
	location   *LocationHandler

	// Batch 5
	staff *StaffHandler

	// Batch 6
	me *MeHandler

	// Batch 11 — 课程分类 / 课程模板 / 课程场次
	courseCategory *CourseCategoryHandler
	courseTemplate *CourseTemplateHandler
	classSession   *ClassSessionHandler

	// Batch 12a — 门店资源
	locationResource *LocationResourceHandler

	// Batch 12b — 循环排课
	recurringSchedule *RecurringScheduleHandler

	// Batch 13a — 学员档案 + 标签
	learner *LearnerHandler

	// Batch 13b — 权益产品 + 发放
	entitlement *EntitlementHandler

	// Batch 13c — 预约下单 + 代取消 + 预约策略
	booking *BookingHandler

	// Batch 13d — 候补
	waitlist *WaitlistHandler

	// Batch 17 — 基础运营看板
	report *ReportHandler
}

// NewHandler 创建品牌 Handler
func NewHandler(
	brandSvc *brandapp.Service,
	brandUserSvc *user.BrandUserService,
	appUserSvc *user.AppUserService,
	trainingSvc *training.Service,
	jwtSvc *cache.Service,
	onboarding *OnboardingHandler,
	profile *ProfileHandler,
	location *LocationHandler,
	staff *StaffHandler,
	me *MeHandler,
	courseCategory *CourseCategoryHandler,
	courseTemplate *CourseTemplateHandler,
	classSession *ClassSessionHandler,
	locationResource *LocationResourceHandler,
	recurringSchedule *RecurringScheduleHandler,
	learner *LearnerHandler,
	entitlement *EntitlementHandler,
	booking *BookingHandler,
	waitlist *WaitlistHandler,
	report *ReportHandler,
) *Handler {
	return &Handler{
		brandSvc:          brandSvc,
		brandUserSvc:      brandUserSvc,
		appUserSvc:        appUserSvc,
		trainingSvc:       trainingSvc,
		jwtSvc:            jwtSvc,
		validator:         validation.New(),
		onboarding:        onboarding,
		profile:           profile,
		location:          location,
		staff:             staff,
		me:                me,
		courseCategory:    courseCategory,
		courseTemplate:    courseTemplate,
		classSession:      classSession,
		locationResource:  locationResource,
		recurringSchedule: recurringSchedule,
		learner:           learner,
		entitlement:       entitlement,
		booking:           booking,
		waitlist:          waitlist,
		report:            report,
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// 公开路由
	public := r.Group("")
	{
		public.POST("/login", h.login)
	}

	// 需要认证的路由
	auth := r.Group("")
	auth.Use(middleware.JWTAuth(h.jwtSvc, "brand"))
	{
		// Deprecated: 用户管理（Batch 1 入口，Batch 5+ 请使用 /staff）。
		auth.POST("/users", h.createUser)
		auth.GET("/users", h.listUsers)
		auth.GET("/users/:id", h.getUser)

		// 训练记录
		auth.GET("/trainings", h.listTrainings)

		// Batch 11 — 课程分类 / 课程模板（替换 legacy 健身 /courses）/ 课程场次
		if h.courseCategory != nil {
			h.courseCategory.RegisterRoutes(auth)
		}
		if h.courseTemplate != nil {
			h.courseTemplate.RegisterRoutes(auth)
		}
		if h.classSession != nil {
			h.classSession.RegisterRoutes(auth)
		}
		if h.locationResource != nil {
			h.locationResource.RegisterRoutes(auth)
		}
		if h.recurringSchedule != nil {
			h.recurringSchedule.RegisterRoutes(auth)
		}
		if h.learner != nil {
			h.learner.RegisterRoutes(auth)
		}
		if h.entitlement != nil {
			h.entitlement.RegisterRoutes(auth)
		}
		if h.booking != nil {
			h.booking.RegisterRoutes(auth)
		}
		if h.waitlist != nil {
			h.waitlist.RegisterRoutes(auth)
		}
		if h.report != nil {
			h.report.RegisterRoutes(auth)
		}

		// Batch 4 — 品牌资料 / onboarding / 门店
		if h.profile != nil {
			h.profile.RegisterRoutes(auth)
		}
		if h.onboarding != nil {
			h.onboarding.RegisterRoutes(auth)
		}
		if h.location != nil {
			h.location.RegisterRoutes(auth)
		}
		if h.staff != nil {
			h.staff.RegisterRoutes(auth)
		}
		if h.me != nil {
			h.me.RegisterRoutes(auth)
		}
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	User         *LoginUserInfo `json:"user"`
}

// LoginUserInfo 登录用户信息
type LoginUserInfo struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	BrandID int64  `json:"brand_id"`
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	accessToken, refreshToken, err := h.brandUserSvc.Login(c.Request.Context(), domainuser.LoginBrandUserInput{
		Phone:    req.Phone,
		Password: req.Password,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	// 获取用户信息
	brandUser, err := h.brandUserSvc.GetUserByPhone(c.Request.Context(), req.Phone)
	if err != nil {
		// 用户信息获取失败不影响登录成功
		response.Success(c, LoginResponse{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		})
		return
	}

	response.Success(c, LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: &LoginUserInfo{
			ID:      brandUser.ID,
			Name:    brandUser.Name,
			Phone:   brandUser.Phone,
			BrandID: brandUser.BrandID,
		},
	})
}

// CreateUserRequest
type CreateUserRequest struct {
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required,min=6"`
	Name     string `json:"name" validate:"required,min=2"`
}

func (h *Handler) createUser(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.brandUserSvc.CreateUser(c.Request.Context(), domainuser.CreateBrandUserInput{
		BrandID:  brandID,
		Phone:    req.Phone,
		Password: req.Password,
		Name:     req.Name,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listUsers(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.appUserSvc.ListUsersByBrand(c.Request.Context(), brandID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

func (h *Handler) getUser(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的用户 ID"))
		return
	}

	result, err := h.appUserSvc.GetUser(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listTrainings(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.trainingSvc.ListByBrand(c.Request.Context(), brandID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

// 确保使用了 trainingdomain（避免未使用导入错误）
var _ = trainingdomain.CreateRecordInput{}

// 确保使用了 domainuser（避免未使用导入错误）
var _ = domainuser.CreateBrandUserInput{}

// 确保使用了 http（避免未使用导入错误）
var _ = http.StatusOK
