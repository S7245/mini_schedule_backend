package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appTpl "github.com/zkw/mini-schedule/backend/internal/application/coursetemplate"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// CourseTemplateHandler brand 端课程模板接口（route /courses，替换 legacy 健身课程）。
type CourseTemplateHandler struct {
	svc *appTpl.Service
}

// NewCourseTemplateHandler 创建 handler。
func NewCourseTemplateHandler(svc *appTpl.Service) *CourseTemplateHandler {
	return &CourseTemplateHandler{svc: svc}
}

// RegisterRoutes 注册 6 个 endpoint。
func (h *CourseTemplateHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/courses", h.list)
	g.GET("/courses/:id", h.get)
	g.POST("/courses", h.create)
	g.PATCH("/courses/:id", h.update)
	g.PATCH("/courses/:id/status", h.updateStatus)
	g.DELETE("/courses/:id", h.delete)
}

func (h *CourseTemplateHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	categoryID, _ := strconv.ParseInt(c.DefaultQuery("category_id", "0"), 10, 64)

	items, total, err := h.svc.List(c.Request.Context(), appTpl.ListInput{
		BrandID:    brandID,
		ActorID:    actorID,
		Status:     c.DefaultQuery("status", "all"),
		Q:          c.Query("q"),
		CategoryID: categoryID,
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

func (h *CourseTemplateHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseCourseID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	t, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, t)
}

type courseBody struct {
	Title             string  `json:"title"`
	Description        string  `json:"description"`
	CoverURL           string  `json:"cover_url"`
	LevelLabel         string  `json:"level_label"`
	DurationMin        int     `json:"duration_min"`
	DefaultCapacity    int     `json:"default_capacity"`
	ShowInMiniProgram  *bool   `json:"show_in_mini_program"`
	CategoryIDs        []int64 `json:"category_ids"`
	LocationIDs        []int64 `json:"location_ids"`
}

func (h *CourseTemplateHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body courseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	show := true
	if body.ShowInMiniProgram != nil {
		show = *body.ShowInMiniProgram
	}
	t, err := h.svc.Create(c.Request.Context(), appTpl.CreateInput{
		BrandID:           brandID,
		ActorID:           actorID,
		Title:             body.Title,
		Description:       body.Description,
		CoverURL:          body.CoverURL,
		LevelLabel:        body.LevelLabel,
		DurationMin:       body.DurationMin,
		DefaultCapacity:   body.DefaultCapacity,
		ShowInMiniProgram: show,
		CategoryIDs:       body.CategoryIDs,
		LocationIDs:       body.LocationIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": t})
}

type updateCourseBody struct {
	Title             *string  `json:"title"`
	Description        *string  `json:"description"`
	CoverURL           *string  `json:"cover_url"`
	LevelLabel         *string  `json:"level_label"`
	DurationMin        *int     `json:"duration_min"`
	DefaultCapacity    *int     `json:"default_capacity"`
	ShowInMiniProgram  *bool    `json:"show_in_mini_program"`
	CategoryIDs        *[]int64 `json:"category_ids"`
	LocationIDs        *[]int64 `json:"location_ids"`
}

func (h *CourseTemplateHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseCourseID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateCourseBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	t, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, appTpl.UpdateInput{
		Title:             body.Title,
		Description:       body.Description,
		CoverURL:          body.CoverURL,
		LevelLabel:        body.LevelLabel,
		DurationMin:       body.DurationMin,
		DefaultCapacity:   body.DefaultCapacity,
		ShowInMiniProgram: body.ShowInMiniProgram,
		CategoryIDs:       body.CategoryIDs,
		LocationIDs:       body.LocationIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, t)
}

func (h *CourseTemplateHandler) updateStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseCourseID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	t, err := h.svc.UpdateStatus(c.Request.Context(), brandID, actorID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, t)
}

func (h *CourseTemplateHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseCourseID(c)
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

func parseCourseID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrCourseNotFound, "课程模板不存在", 404)
	}
	return id, nil
}
