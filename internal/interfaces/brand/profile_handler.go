package brand

import (
	"github.com/gin-gonic/gin"

	appBrandProfile "github.com/zkw/mini-schedule/backend/internal/application/brandprofile"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// ProfileHandler brand 端品牌资料接口。
type ProfileHandler struct {
	svc *appBrandProfile.Service
}

// NewProfileHandler 创建 handler。
func NewProfileHandler(svc *appBrandProfile.Service) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

// RegisterRoutes 注册 GET/PATCH /profile。
func (h *ProfileHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/profile", h.get)
	g.PATCH("/profile", h.patch)
}

func (h *ProfileHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	p, err := h.svc.GetProfile(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

// patchProfileBody 是白名单字段，未列出的字段（name / contact_phone / contact_name）即便客户端传入也被丢弃。
type patchProfileBody struct {
	LogoURL      *string `json:"logo_url"`
	Description  *string `json:"description"`
	IndustryType *string `json:"industry_type"`
	BrandCode    *string `json:"brand_code"`
	ContactEmail *string `json:"contact_email"`
}

func (h *ProfileHandler) patch(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body patchProfileBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.UpdateProfile(c.Request.Context(), brandID, actorID, appBrandProfile.Input{
		LogoURL:      body.LogoURL,
		Description:  body.Description,
		IndustryType: body.IndustryType,
		BrandCode:    body.BrandCode,
		ContactEmail: body.ContactEmail,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}
