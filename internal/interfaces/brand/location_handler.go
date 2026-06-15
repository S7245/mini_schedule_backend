package brand

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appLocation "github.com/zkw/mini-schedule/backend/internal/application/location"
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
	actorID := middleware.GetUserID(c)
	var body createLocationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	loc, err := h.svc.Create(c.Request.Context(), appLocation.CreateInput{
		BrandID: brandID,
		ActorID: actorID,
		Name:    body.Name,
		Address: body.Address,
		Phone:   body.Phone,
		Remark:  body.Remark,
	})
	if err != nil {
		// review #6：QUOTA_EXCEEDED 的 current/max 已通过 AppError.Details 挂载，
		// response.Error 会统一序列化进 Response.Data，无需再手写非标准 envelope。
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
	q := c.Query("q")

	items, total, err := h.svc.List(c.Request.Context(), appLocation.ListInput{
		BrandID:  brandID,
		ActorID:  middleware.GetUserID(c),
		Status:   status,
		Q:        q,
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
	actorID := middleware.GetUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	loc, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
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
	actorID := middleware.GetUserID(c)
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
	loc, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, appLocation.UpdateInput{
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
	actorID := middleware.GetUserID(c)
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
	loc, err := h.svc.UpdateStatus(c.Request.Context(), brandID, actorID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, loc)
}

func (h *LocationHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLocationNotFound, "门店不存在", 404))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), brandID, actorID, id); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// keep imports tidy when some are conditionally used.
var _ = errors.Is
