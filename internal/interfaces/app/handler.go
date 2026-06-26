package app

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zkw/mini-schedule/backend/internal/application/course"
	"github.com/zkw/mini-schedule/backend/internal/application/learnerbooking"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	learnerdomain "github.com/zkw/mini-schedule/backend/internal/domain/learner"
	trainingdomain "github.com/zkw/mini-schedule/backend/internal/domain/training"
	domainuser "github.com/zkw/mini-schedule/backend/internal/domain/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
	"github.com/zkw/mini-schedule/backend/pkg/validation"
)

// Handler C 端用户 Handler
type Handler struct {
	appUserSvc     *user.AppUserService
	courseSvc      *course.Service
	trainingSvc    *training.Service
	jwtSvc         *cache.Service
	learnerRepo    learnerdomain.Repository // 登录桥接 by-openid find-or-create profile（Batch 14a）。
	learnerBookSvc *learnerbooking.Service  // C 端自助预约/取消/课程表（Batch 14a）。
	validator      *validation.Validator
}

// NewHandler 创建 C 端 Handler
func NewHandler(
	appUserSvc *user.AppUserService,
	courseSvc *course.Service,
	trainingSvc *training.Service,
	jwtSvc *cache.Service,
	learnerRepo learnerdomain.Repository,
	learnerBookSvc *learnerbooking.Service,
) *Handler {
	return &Handler{
		appUserSvc:     appUserSvc,
		courseSvc:      courseSvc,
		trainingSvc:    trainingSvc,
		jwtSvc:         jwtSvc,
		learnerRepo:    learnerRepo,
		learnerBookSvc: learnerBookSvc,
		validator:      validation.New(),
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// 公开路由（微信登录等）
	public := r.Group("")
	{
		public.POST("/auth/wechat-login", h.wechatLogin)
	}

	// 需要认证的路由
	auth := r.Group("")
	auth.Use(middleware.JWTAuth(h.jwtSvc, "app"))
	{
		auth.GET("/profile", h.getProfile)
		auth.PUT("/profile", h.updateProfile)

		// 课程
		auth.GET("/courses", h.listCourses)
		auth.GET("/courses/:id", h.getCourse)

		// 训练记录
		auth.POST("/trainings", h.createTraining)
		auth.GET("/trainings", h.listMyTrainings)

		// 自助预约（Batch 14a）：课程表只读 + 下单 + 我的预约 + 自助取消 + 权益预览。
		auth.GET("/class-sessions", h.listClassSessions)
		auth.GET("/class-sessions/:id", h.getClassSession)
		auth.POST("/bookings", h.createBooking)
		auth.GET("/bookings", h.listMyBookings)
		auth.GET("/bookings/usable-entitlements", h.usableEntitlements)
		auth.POST("/bookings/:id/cancel", h.cancelBooking)

		// 自助增量（Batch 14b）：我的权益 + 加入/我的/取消候补。上课记录复用 GET /bookings?status=attended,no_show。
		auth.GET("/entitlements", h.listEntitlements)
		auth.POST("/waitlist", h.joinWaitlist)
		auth.GET("/waitlist", h.listMyWaitlist)
		auth.POST("/waitlist/:id/cancel", h.cancelWaitlist)
	}
}

// WechatLoginRequest 微信登录请求
type WechatLoginRequest struct {
	BrandID  int64  `json:"brand_id" validate:"required,gt=0"`
	Code     string `json:"code" validate:"required"`
	Nickname string `json:"nickname" validate:"omitempty,max=50"`
}

// WechatLoginResponse 微信登录响应
type WechatLoginResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *AppUserInfo `json:"user"`
	IsNewUser    bool         `json:"is_new_user"`
}

// AppUserInfo C 端用户信息
type AppUserInfo struct {
	ID        int64  `json:"id"`
	BrandID   int64  `json:"brand_id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
	VIPLevel  string `json:"vip_level"`
}

func (h *Handler) wechatLogin(c *gin.Context) {
	var req WechatLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	// 实际流程：用 code 调微信 API 获取 openid
	// openID, err := wechatSvc.Code2Session(req.Code)
	// 这里用 code 作为 openid 的占位（开发环境）
	openID := "dev_" + req.Code

	// 获取或创建用户
	existingUser, err := h.appUserSvc.GetUserByOpenID(c.Request.Context(), req.BrandID, openID)
	isNewUser := false
	if err != nil {
		// 用户不存在，创建新用户
		newUser, createErr := h.appUserSvc.CreateUser(c.Request.Context(), domainuser.CreateAppUserInput{
			BrandID:  req.BrandID,
			OpenID:   openID,
			Nickname: req.Nickname,
		})
		if createErr != nil {
			response.Error(c, createErr)
			return
		}
		existingUser = newUser
		isNewUser = true
	}

	// 桥接（Batch 14a）：find-or-create learner_identity(by openid)+brand_learner_profile(by brand+identity)，
	// 把 brand_learner_profile_id 带入 token，使 C 端自助预约与 brand 侧操作同一 profile。
	// app_user 与 learner profile 双轨并存（app_user 留 legacy 课程/训练页）。
	profile, perr := h.learnerRepo.FindOrCreateProfileByOpenID(c.Request.Context(), req.BrandID, openID, req.Nickname)
	if perr != nil {
		response.Error(c, perr)
		return
	}

	// 生成 JWT token
	payload := cache.TokenPayload{
		UserID:    existingUser.ID,
		BrandID:   existingUser.BrandID,
		UserType:  "app",
		ProfileID: profile.ID,
	}

	accessToken, err := h.jwtSvc.GenerateToken(payload)
	if err != nil {
		response.Error(c, err)
		return
	}

	refreshToken, err := h.jwtSvc.GenerateRefreshToken(payload)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, WechatLoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User: &AppUserInfo{
			ID:        existingUser.ID,
			BrandID:   existingUser.BrandID,
			Nickname:  existingUser.Nickname,
			AvatarURL: existingUser.AvatarURL,
			VIPLevel:  string(existingUser.VIPLevel),
		},
		IsNewUser: isNewUser,
	})
}

func (h *Handler) getProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	result, err := h.appUserSvc.GetUser(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// UpdateProfileRequest
type UpdateProfileRequest struct {
	Nickname  string `json:"nickname" validate:"omitempty,max=50"`
	AvatarURL string `json:"avatar_url" validate:"omitempty,url"`
}

func (h *Handler) updateProfile(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.appUserSvc.UpdateUser(c.Request.Context(), userID, req.Nickname, req.AvatarURL)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listCourses(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// C 端用户只能看到已发布的课程
	// brandID 从用户信息中获取
	brandID := middleware.GetBrandID(c)
	results, total, err := h.courseSvc.ListPublishedCourses(c.Request.Context(), brandID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

func (h *Handler) getCourse(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的课程 ID"))
		return
	}

	result, err := h.courseSvc.GetCourse(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// CreateTrainingRequest
type CreateTrainingRequest struct {
	CourseID    int64   `json:"course_id" validate:"required,gt=0"`
	DurationMin int     `json:"duration_min" validate:"required,gt=0"`
	Calories    float64 `json:"calories" validate:"omitempty,gte=0"`
	Notes       string  `json:"notes" validate:"omitempty,max=500"`
}

func (h *Handler) createTraining(c *gin.Context) {
	userID := middleware.GetUserID(c)
	brandID := middleware.GetBrandID(c)

	var req CreateTrainingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.trainingSvc.CreateRecord(c.Request.Context(), trainingdomain.CreateRecordInput{
		UserID:      userID,
		BrandID:     brandID,
		CourseID:    req.CourseID,
		DurationMin: req.DurationMin,
		Calories:    req.Calories,
		Notes:       req.Notes,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listMyTrainings(c *gin.Context) {
	userID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.trainingSvc.ListByUser(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

// ---- 自助预约（Batch 14a）----

func (h *Handler) listClassSessions(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.learnerBookSvc.ListSessions(c.Request.Context(), brandID, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) getClassSession(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的场次 ID"))
		return
	}
	result, err := h.learnerBookSvc.GetSession(c.Request.Context(), brandID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// CreateBookingRequest 自助下单请求。
type CreateBookingRequest struct {
	ClassSessionID int64 `json:"class_session_id" validate:"required,gt=0"`
}

func (h *Handler) createBooking(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)

	var req CreateBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.learnerBookSvc.Book(c.Request.Context(), brandID, profileID, req.ClassSessionID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) listMyBookings(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.learnerBookSvc.ListMyBookings(c.Request.Context(), brandID, profileID, status, page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) usableEntitlements(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	sessionID, err := strconv.ParseInt(c.Query("class_session_id"), 10, 64)
	if err != nil || sessionID <= 0 {
		response.Error(c, response.ErrInvalidRequest("无效的场次 ID"))
		return
	}
	items, err := h.learnerBookSvc.UsableEntitlements(c.Request.Context(), brandID, profileID, sessionID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

// CancelBookingRequest 自助取消请求（reason 可选）。
type CancelBookingRequest struct {
	Reason string `json:"reason" validate:"omitempty,max=500"`
}

func (h *Handler) cancelBooking(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的预约 ID"))
		return
	}
	var req CancelBookingRequest
	// reason 可选：body 为空 / 非法 JSON 时按无原因处理。
	_ = c.ShouldBindJSON(&req)
	if verr := h.validator.Struct(req); verr != nil {
		response.Error(c, h.validator.InvalidRequest(c, verr))
		return
	}
	result, err := h.learnerBookSvc.CancelMyBooking(c.Request.Context(), brandID, profileID, id, req.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// ---- 自助增量（Batch 14b）----

func (h *Handler) listEntitlements(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	items, err := h.learnerBookSvc.ListMyEntitlements(c.Request.Context(), brandID, profileID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

// JoinWaitlistRequest 加入候补请求。
type JoinWaitlistRequest struct {
	ClassSessionID int64 `json:"class_session_id" validate:"required,gt=0"`
}

func (h *Handler) joinWaitlist(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	var req JoinWaitlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.learnerBookSvc.JoinWaitlist(c.Request.Context(), brandID, profileID, req.ClassSessionID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) listMyWaitlist(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	items, err := h.learnerBookSvc.ListMyWaitlist(c.Request.Context(), brandID, profileID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

func (h *Handler) cancelWaitlist(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	profileID := middleware.GetProfileID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的候补 ID"))
		return
	}
	result, err := h.learnerBookSvc.CancelMyWaitlist(c.Request.Context(), brandID, profileID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

// 确保使用了 domainuser（避免未使用导入错误）
var _ = domainuser.CreateAppUserInput{}

// 确保使用了 http（避免未使用导入错误）
var _ = http.StatusOK
