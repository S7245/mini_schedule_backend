package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	brandapp "github.com/zkw/mini-schedule/backend/internal/application/brand"
	"github.com/zkw/mini-schedule/backend/internal/application/course"
	"github.com/zkw/mini-schedule/backend/internal/application/training"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	coursedomain "github.com/zkw/mini-schedule/backend/internal/domain/course"
	domainuser "github.com/zkw/mini-schedule/backend/internal/domain/user"
	trainingdomain "github.com/zkw/mini-schedule/backend/internal/domain/training"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// Handler 品牌商家后台 Handler
type Handler struct {
	brandSvc     *brandapp.Service
	brandUserSvc *user.BrandUserService
	appUserSvc   *user.AppUserService
	courseSvc    *course.Service
	trainingSvc  *training.Service
	jwtSvc       *cache.Service
	validator    *validator.Validate
}

// NewHandler 创建品牌 Handler
func NewHandler(
	brandSvc *brandapp.Service,
	brandUserSvc *user.BrandUserService,
	appUserSvc *user.AppUserService,
	courseSvc *course.Service,
	trainingSvc *training.Service,
	jwtSvc *cache.Service,
) *Handler {
	return &Handler{
		brandSvc:     brandSvc,
		brandUserSvc: brandUserSvc,
		appUserSvc:   appUserSvc,
		courseSvc:    courseSvc,
		trainingSvc:  trainingSvc,
		jwtSvc:       jwtSvc,
		validator:    validator.New(),
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
		// 用户管理
		auth.POST("/users", h.createUser)
		auth.GET("/users", h.listUsers)
		auth.GET("/users/:id", h.getUser)

		// 课程管理
		auth.POST("/courses", h.createCourse)
		auth.GET("/courses", h.listCourses)
		auth.GET("/courses/:id", h.getCourse)
		auth.PUT("/courses/:id", h.updateCourse)
		auth.DELETE("/courses/:id", h.deleteCourse)
		auth.PATCH("/courses/:id/status", h.updateCourseStatus)

		// 训练记录
		auth.GET("/trainings", h.listTrainings)
	}
}

// LoginRequest 登录请求
type LoginRequest struct {
	Phone    string `json:"phone" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
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
		response.Error(c, response.ErrInvalidRequest(err.Error()))
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
		response.Error(c, response.ErrInvalidRequest(err.Error()))
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

// CreateCourseRequest
type CreateCourseRequest struct {
	Title       string `json:"title" validate:"required,min=2,max=200"`
	Description string `json:"description" validate:"omitempty,max=2000"`
	CoverURL    string `json:"cover_url" validate:"omitempty,url"`
	Difficulty  string `json:"difficulty" validate:"required,oneof=beginner intermediate advanced"`
	DurationMin int    `json:"duration_min" validate:"required,gt=0"`
	Type        string `json:"type" validate:"required,oneof=strength cardio flexibility hiit"`
}

func (h *Handler) createCourse(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	var req CreateCourseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, response.ErrInvalidRequest(err.Error()))
		return
	}

	result, err := h.courseSvc.CreateCourse(c.Request.Context(), coursedomain.CreateCourseInput{
		BrandID:     brandID,
		Title:       req.Title,
		Description: req.Description,
		CoverURL:    req.CoverURL,
		Difficulty:  coursedomain.Difficulty(req.Difficulty),
		DurationMin: req.DurationMin,
		Type:        coursedomain.CourseType(req.Type),
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listCourses(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.courseSvc.ListCoursesByBrand(c.Request.Context(), brandID, page, pageSize)
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

func (h *Handler) updateCourse(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的课程 ID"))
		return
	}

	var req coursedomain.UpdateCourseInput
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	result, err := h.courseSvc.UpdateCourse(c.Request.Context(), id, req)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) deleteCourse(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的课程 ID"))
		return
	}

	if err := h.courseSvc.DeleteCourse(c.Request.Context(), id); err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessNoData(c)
}

func (h *Handler) updateCourseStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的课程 ID"))
		return
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=draft published archived"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.courseSvc.UpdateCourseStatus(c.Request.Context(), id, coursedomain.Status(req.Status)); err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessNoData(c)
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
