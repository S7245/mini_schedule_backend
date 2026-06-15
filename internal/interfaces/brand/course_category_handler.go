package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appCategory "github.com/zkw/mini-schedule/backend/internal/application/coursecategory"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// CourseCategoryHandler brand 端课程分类接口。
type CourseCategoryHandler struct {
	svc *appCategory.Service
}

// NewCourseCategoryHandler 创建 handler。
func NewCourseCategoryHandler(svc *appCategory.Service) *CourseCategoryHandler {
	return &CourseCategoryHandler{svc: svc}
}

// RegisterRoutes 注册 3 个 endpoint。
func (h *CourseCategoryHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/course-categories", h.list)
	g.POST("/course-categories", h.create)
	g.PATCH("/course-categories/:id", h.update)
}

func (h *CourseCategoryHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	status := c.DefaultQuery("status", "all")
	items, err := h.svc.List(c.Request.Context(), brandID, actorID, status)
	if err != nil {
		response.Error(c, err)
		return
	}
	// 不分页，但保持 items 包裹结构（前端 .map() 防御：返 [] 而非 null）。
	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		out = append(out, gin.H{
			"id":                   it.ID,
			"name":                 it.Name,
			"color":                it.Color,
			"icon":                 it.Icon,
			"sort_order":           it.SortOrder,
			"show_in_mini_program": it.ShowInMiniProgram,
			"status":               it.Status,
		})
	}
	response.Success(c, gin.H{"items": out})
}

type createCategoryBody struct {
	Name              string  `json:"name"`
	Color             string  `json:"color"`
	Icon              string  `json:"icon"`
	SortOrder         int     `json:"sort_order"`
	ShowInMiniProgram *bool   `json:"show_in_mini_program"`
}

func (h *CourseCategoryHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createCategoryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	show := true
	if body.ShowInMiniProgram != nil {
		show = *body.ShowInMiniProgram
	}
	cat, err := h.svc.Create(c.Request.Context(), appCategory.CreateInput{
		BrandID:           brandID,
		ActorID:           actorID,
		Name:              body.Name,
		Color:             body.Color,
		Icon:              body.Icon,
		SortOrder:         body.SortOrder,
		ShowInMiniProgram: show,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": cat})
}

type updateCategoryBody struct {
	Name              *string `json:"name"`
	Color             *string `json:"color"`
	Icon              *string `json:"icon"`
	SortOrder         *int    `json:"sort_order"`
	ShowInMiniProgram *bool   `json:"show_in_mini_program"`
	Status            *string `json:"status"`
}

func (h *CourseCategoryHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrCategoryNotFound, "课程分类不存在", 404))
		return
	}
	var body updateCategoryBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	cat, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, appCategory.UpdateInput{
		Name:              body.Name,
		Color:             body.Color,
		Icon:              body.Icon,
		SortOrder:         body.SortOrder,
		ShowInMiniProgram: body.ShowInMiniProgram,
		Status:            body.Status,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, cat)
}
