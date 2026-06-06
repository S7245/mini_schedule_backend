package brand

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appLocation "github.com/zkw/mini-schedule/backend/internal/application/location"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// LocationHandler brand 端门店接口。
type LocationHandler struct {
	svc *appLocation.Service
}

// NewLocationHandler 创建 handler。
func NewLocationHandler(svc *appLocation.Service) *LocationHandler {
	return &LocationHandler{svc: svc}
}

// RegisterRoutes 注册 6 个 endpoint。
func (h *LocationHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/locations", h.list)
	g.GET("/locations/:id", h.get)
	g.POST("/locations", h.create)
	g.PATCH("/locations/:id", h.update)
	g.PATCH("/locations/:id/status", h.updateStatus)
	g.DELETE("/locations/:id", h.delete)
}

type createLocationBody struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Remark  string `json:"remark"`
}

func (h *LocationHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	var body createLocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	loc, err := h.svc.Create(c.Request.Context(), appLocation.CreateInput{
		BrandID: brandID,
		Name:    body.Name,
		Address: body.Address,
		Phone:   body.Phone,
		Remark:  body.Remark,
	})
	if err != nil {
		// 如果是 QUOTA_EXCEEDED，附带 current/max 详情
		if cur, max, ok := persistence.QuotaDetailsFromError(err); ok {
			ae := apperr.GetAppError(err)
			c.JSON(ae.HTTPStatus, gin.H{
				"code":    string(ae.Code),
				"message": ae.Message,
				"data": gin.H{
					"current": cur,
					"max":     max,
				},
			})
			return
		}
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"code":    "OK",
		"message": "created",
		"data":    loc,
	})
}

func (h *LocationHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.DefaultQuery("status", "all")

	items, total, err := h.svc.List(c.Request.Context(), appLocation.ListInput{
		BrandID:  brandID,
		Status:   status,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *LocationHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	loc, err := h.svc.Get(c.Request.Context(), brandID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, loc)
}

type updateLocationBody struct {
	Name    *string `json:"name"`
	Address *string `json:"address"`
	Phone   *string `json:"phone"`
	Remark  *string `json:"remark"`
}

func (h *LocationHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	var body updateLocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	loc, err := h.svc.Update(c.Request.Context(), brandID, id, appLocation.UpdateInput{
		Name:    body.Name,
		Address: body.Address,
		Phone:   body.Phone,
		Remark:  body.Remark,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, loc)
}

type updateLocationStatusBody struct {
	Status string `json:"status"`
}

func (h *LocationHandler) updateStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	var body updateLocationStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	loc, err := h.svc.UpdateStatus(c.Request.Context(), brandID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, loc)
}

func (h *LocationHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), brandID, id); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// keep imports tidy when some are conditionally used.
var _ = errors.Is
