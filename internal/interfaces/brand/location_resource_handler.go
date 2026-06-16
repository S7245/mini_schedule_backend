package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appResource "github.com/zkw/mini-schedule/backend/internal/application/locationresource"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// LocationResourceHandler brand 端门店资源接口。
type LocationResourceHandler struct {
	svc *appResource.Service
}

// NewLocationResourceHandler 创建 handler。
func NewLocationResourceHandler(svc *appResource.Service) *LocationResourceHandler {
	return &LocationResourceHandler{svc: svc}
}

// RegisterRoutes 注册 5 个 endpoint。
func (h *LocationResourceHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/location-resources", h.list)
	g.GET("/location-resources/:id", h.get)
	g.POST("/location-resources", h.create)
	g.PATCH("/location-resources/:id", h.update)
	g.DELETE("/location-resources/:id", h.delete)
}

func parseResourceID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrResourceNotFound, "资源不存在", 404)
	}
	return id, nil
}

func (h *LocationResourceHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	locationID, _ := strconv.ParseInt(c.DefaultQuery("location_id", "0"), 10, 64)

	items, total, err := h.svc.List(c.Request.Context(), appResource.ListInput{
		BrandID:    brandID,
		ActorID:    actorID,
		LocationID: locationID,
		Status:     c.Query("status"),
		Page:       page,
		PageSize:   pageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *LocationResourceHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseResourceID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	res, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, res)
}

type createResourceBody struct {
	LocationID int64  `json:"location_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Capacity   int    `json:"capacity"`
	Remark     string `json:"remark"`
}

func (h *LocationResourceHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createResourceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	res, err := h.svc.Create(c.Request.Context(), appResource.CreateInput{
		BrandID:    brandID,
		ActorID:    actorID,
		LocationID: body.LocationID,
		Name:       body.Name,
		Type:       body.Type,
		Capacity:   body.Capacity,
		Remark:     body.Remark,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": res})
}

type updateResourceBody struct {
	Name     *string `json:"name"`
	Type     *string `json:"type"`
	Capacity *int    `json:"capacity"`
	Status   *string `json:"status"`
	Remark   *string `json:"remark"`
}

func (h *LocationResourceHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseResourceID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateResourceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	res, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, appResource.UpdateInput{
		Name:     body.Name,
		Type:     body.Type,
		Capacity: body.Capacity,
		Status:   body.Status,
		Remark:   body.Remark,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, res)
}

func (h *LocationResourceHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseResourceID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), brandID, actorID, id); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
