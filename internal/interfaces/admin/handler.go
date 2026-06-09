package admin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zkw/mini-schedule/backend/internal/application/brand"
	commercialapp "github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/application/user"
	branddomain "github.com/zkw/mini-schedule/backend/internal/domain/brand"
	domainuser "github.com/zkw/mini-schedule/backend/internal/domain/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
	"github.com/zkw/mini-schedule/backend/pkg/validation"
)

// Handler 平台管理后台 Handler
type Handler struct {
	brandSvc      *brand.Service
	commercialSvc *commercialapp.Service
	adminUserSvc  *user.AdminUserService
	jwtSvc        *cache.Service
	validator     *validation.Validator

	// Batch 5
	system *SystemHandler
}

// NewHandler 创建管理端 Handler.
//
// system 可为 nil；brand 进程（providePublicHandler）只用 RegisterPublicRoutes，
// 不需要 system endpoint，故注入 nil。
func NewHandler(
	brandSvc *brand.Service,
	commercialSvc *commercialapp.Service,
	adminUserSvc *user.AdminUserService,
	jwtSvc *cache.Service,
	system *SystemHandler,
) *Handler {
	return &Handler{
		brandSvc:      brandSvc,
		commercialSvc: commercialSvc,
		adminUserSvc:  adminUserSvc,
		jwtSvc:        jwtSvc,
		validator:     validation.New(),
		system:        system,
	}
}

// RegisterRoutes 注册路由
func (h *Handler) RegisterRoutes(r *gin.RouterGroup) {
	// 公开路由
	public := r.Group("")
	{
		// @Summary 管理员登录
		// @Description 使用用户名和密码登录管理后台
		// @Tags 认证
		// @Accept json
		// @Produce json
		// @Param body body LoginRequest true "登录信息"
		// @Success 200 {object} response.Response{data=LoginResponse} "成功"
		// @Failure 400 {object} response.Response "参数错误"
		// @Failure 401 {object} response.Response "认证失败"
		// @Router /api/v1/admin/login [post]
		public.POST("/login", h.login)

		// @Summary 管理员退出登录
		// @Description 清理管理后台登录 Cookie
		// @Tags 认证
		// @Produce json
		// @Success 200 {object} response.Response "成功"
		// @Router /api/v1/admin/logout [post]
		public.POST("/logout", h.logout)
	}

	// 需要认证的路由
	auth := r.Group("")
	auth.Use(middleware.JWTAuth(h.jwtSvc, "admin"))
	{
		// @Summary 创建品牌
		// @Description 创建新品牌入驻
		// @Tags 品牌管理
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Param body body CreateBrandRequest true "品牌信息"
		// @Success 200 {object} response.Response{data=branddomain.Brand} "成功"
		// @Failure 400 {object} response.Response "参数错误"
		// @Failure 401 {object} response.Response "未认证"
		// @Router /api/v1/admin/brands [post]
		auth.POST("/brands", h.createBrand)

		// @Summary 品牌列表
		// @Description 分页获取品牌列表
		// @Tags 品牌管理
		// @Produce json
		// @Security BearerAuth
		// @Param page query int false "页码" default(1)
		// @Param page_size query int false "每页数量" default(20)
		// @Success 200 {object} response.Response{data=response.PageData} "成功"
		// @Failure 401 {object} response.Response "未认证"
		// @Router /api/v1/admin/brands [get]
		auth.GET("/brands", h.listBrands)

		// @Summary 品牌详情
		// @Description 获取单个品牌详情
		// @Tags 品牌管理
		// @Produce json
		// @Security BearerAuth
		// @Param id path int true "品牌 ID"
		// @Success 200 {object} response.Response{data=branddomain.Brand} "成功"
		// @Failure 404 {object} response.Response "品牌不存在"
		// @Router /api/v1/admin/brands/{id} [get]
		auth.GET("/brands/:id", h.getBrand)

		// @Summary 更新品牌
		// @Description 更新品牌信息
		// @Tags 品牌管理
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Param id path int true "品牌 ID"
		// @Param body body UpdateBrandRequest true "品牌信息"
		// @Success 200 {object} response.Response{data=branddomain.Brand} "成功"
		// @Router /api/v1/admin/brands/{id} [put]
		auth.PUT("/brands/:id", h.updateBrand)

		// @Summary 更新品牌状态
		// @Description 启用/禁用/待审核品牌
		// @Tags 品牌管理
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Param id path int true "品牌 ID"
		// @Param body body object{status=string} true "状态"
		// @Success 200 {object} response.Response "成功"
		// @Router /api/v1/admin/brands/{id}/status [patch]
		auth.PATCH("/brands/:id/status", h.updateBrandStatus)

		// @Summary 创建管理员
		// @Description 创建平台管理员账号
		// @Tags 管理员管理
		// @Accept json
		// @Produce json
		// @Security BearerAuth
		// @Param body body CreateAdminRequest true "管理员信息"
		// @Success 200 {object} response.Response{data=domainuser.AdminUser} "成功"
		// @Router /api/v1/admin/admins [post]
		auth.POST("/admins", h.createAdmin)

		// @Summary 管理员列表
		// @Description 分页获取管理员列表
		// @Tags 管理员管理
		// @Produce json
		// @Security BearerAuth
		// @Param page query int false "页码" default(1)
		// @Param page_size query int false "每页数量" default(20)
		// @Success 200 {object} response.Response{data=response.PageData} "成功"
		// @Router /api/v1/admin/admins [get]
		auth.GET("/admins", h.listAdmins)

		h.registerCommercialRoutes(auth)
		if h.system != nil {
			h.system.RegisterRoutes(auth)
		}
	}
}

// LoginRequest
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	User         *AdminUserInfo `json:"user"`
}

// AdminUserInfo 管理员信息
type AdminUserInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

const (
	adminAccessTokenCookieName  = "admin_access_token"
	adminRefreshTokenCookieName = "admin_refresh_token"
)

func isSecureRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	return c.GetHeader("X-Forwarded-Proto") == "https"
}

func setTokenCookies(c *gin.Context, jwtSvc *cache.Service, accessToken, refreshToken string) {
	secure := isSecureRequest(c)

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminAccessTokenCookieName,
		Value:    accessToken,
		Path:     "/",
		MaxAge:   jwtSvc.AccessTokenMaxAge(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminRefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   jwtSvc.RefreshTokenMaxAge(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

func clearTokenCookies(c *gin.Context) {
	secure := isSecureRequest(c)

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminAccessTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminRefreshTokenCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
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

	accessToken, refreshToken, err := h.adminUserSvc.Login(c.Request.Context(), domainuser.LoginAdminUserInput{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	setTokenCookies(c, h.jwtSvc, accessToken, refreshToken)

	// 获取管理员信息
	adminUser, err := h.adminUserSvc.GetUserByUsername(c.Request.Context(), req.Username)
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
		User: &AdminUserInfo{
			ID:       adminUser.ID,
			Username: adminUser.Username,
			Role:     string(adminUser.Role),
		},
	})
}

func (h *Handler) logout(c *gin.Context) {
	clearTokenCookies(c)
	response.SuccessNoData(c)
}

// CreateBrandRequest
type CreateBrandRequest struct {
	Name         string `json:"name" validate:"required,min=2,max=100"`
	LogoURL      string `json:"logo_url" validate:"omitempty,url"`
	ContactName  string `json:"contact_name" validate:"required,min=2"`
	ContactPhone string `json:"contact_phone" validate:"required"`
}

func (h *Handler) createBrand(c *gin.Context) {
	var req CreateBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.brandSvc.CreateBrand(c.Request.Context(), branddomain.CreateBrandInput{
		Name:         req.Name,
		LogoURL:      req.LogoURL,
		ContactName:  req.ContactName,
		ContactPhone: req.ContactPhone,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listBrands(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.brandSvc.ListBrands(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

func (h *Handler) getBrand(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的品牌 ID"))
		return
	}

	result, err := h.brandSvc.GetBrand(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

// UpdateBrandRequest
type UpdateBrandRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=2,max=100"`
	LogoURL     *string `json:"logo_url" validate:"omitempty,url"`
	ContactName *string `json:"contact_name" validate:"omitempty,min=2"`
}

func (h *Handler) updateBrand(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的品牌 ID"))
		return
	}

	var req UpdateBrandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.brandSvc.UpdateBrand(c.Request.Context(), id, branddomain.UpdateBrandInput{
		Name:        req.Name,
		LogoURL:     req.LogoURL,
		ContactName: req.ContactName,
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) updateBrandStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的品牌 ID"))
		return
	}

	var req struct {
		Status string `json:"status" validate:"required,oneof=active inactive pending"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.brandSvc.UpdateBrandStatus(c.Request.Context(), id, branddomain.Status(req.Status)); err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessNoData(c)
}

// CreateAdminRequest
type CreateAdminRequest struct {
	Username string `json:"username" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=6"`
	Role     string `json:"role" validate:"required,oneof=super_admin operator support"`
}

func (h *Handler) createAdmin(c *gin.Context) {
	var req CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}

	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.adminUserSvc.CreateUser(c.Request.Context(), domainuser.CreateAdminUserInput{
		Username: req.Username,
		Password: req.Password,
		Role:     domainuser.Role(req.Role),
	})
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, result)
}

func (h *Handler) listAdmins(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	results, total, err := h.adminUserSvc.ListUsers(c.Request.Context(), page, pageSize)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.SuccessPage(c, results, total, page, pageSize)
}

// 确保使用了 http（避免未使用导入错误）
var _ = http.StatusOK
