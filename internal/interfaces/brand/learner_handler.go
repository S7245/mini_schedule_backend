package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appLearner "github.com/zkw/mini-schedule/backend/internal/application/learner"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// LearnerHandler brand 端学员档案 + 标签接口（Batch 13a）。
type LearnerHandler struct {
	svc *appLearner.Service
}

// NewLearnerHandler 创建 handler。
func NewLearnerHandler(svc *appLearner.Service) *LearnerHandler {
	return &LearnerHandler{svc: svc}
}

// RegisterRoutes 注册学员 + 标签路由。
func (h *LearnerHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/learners", h.list)
	g.GET("/learners/:id", h.get)
	g.POST("/learners", h.create)
	g.PATCH("/learners/:id", h.update)
	g.PATCH("/learners/:id/status", h.updateStatus)
	g.DELETE("/learners/:id", h.delete)

	g.GET("/learner-tags", h.listTags)
	g.POST("/learner-tags", h.createTag)
	g.PATCH("/learner-tags/:id", h.updateTag)
}

func parseLearnerID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
	}
	return id, nil
}

func (h *LearnerHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	locationID, _ := strconv.ParseInt(c.DefaultQuery("primary_location_id", "0"), 10, 64)

	items, total, err := h.svc.List(c.Request.Context(), appLearner.ListInput{
		BrandID:           brandID,
		ActorID:           actorID,
		Status:            c.Query("status"),
		PrimaryLocationID: locationID,
		Query:             c.Query("q"),
		Page:              page,
		PageSize:          pageSize,
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

func (h *LearnerHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseLearnerID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	p, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

type createLearnerBody struct {
	Phone             string  `json:"phone"`
	Nickname          string  `json:"nickname"`
	PrimaryLocationID *int64  `json:"primary_location_id"`
	LearnerNo         string  `json:"learner_no"`
	Remark            string  `json:"remark"`
	TagIDs            []int64 `json:"tag_ids"`
}

func (h *LearnerHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createLearnerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.Create(c.Request.Context(), appLearner.CreateInput{
		BrandID:           brandID,
		ActorID:           actorID,
		Phone:             body.Phone,
		Nickname:          body.Nickname,
		PrimaryLocationID: body.PrimaryLocationID,
		LearnerNo:         body.LearnerNo,
		Remark:            body.Remark,
		TagIDs:            body.TagIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": p})
}

type updateLearnerBody struct {
	Nickname          *string  `json:"nickname"`
	PrimaryLocationID *int64   `json:"primary_location_id"`
	LearnerNo         *string  `json:"learner_no"`
	Remark            *string  `json:"remark"`
	TagIDs            *[]int64 `json:"tag_ids"`
}

func (h *LearnerHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseLearnerID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateLearnerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, appLearner.UpdateInput{
		Nickname:          body.Nickname,
		PrimaryLocationID: body.PrimaryLocationID,
		LearnerNo:         body.LearnerNo,
		Remark:            body.Remark,
		TagIDs:            body.TagIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

type statusBody struct {
	Status string `json:"status"`
}

func (h *LearnerHandler) updateStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseLearnerID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body statusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.UpdateStatus(c.Request.Context(), brandID, actorID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

func (h *LearnerHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseLearnerID(c)
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

// ---- 标签 ----

func (h *LearnerHandler) listTags(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	items, err := h.svc.ListTags(c.Request.Context(), brandID, actorID, c.Query("status"))
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

type createTagBody struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *LearnerHandler) createTag(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createTagBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	tag, err := h.svc.CreateTag(c.Request.Context(), appLearner.CreateTagInput{
		BrandID: brandID, ActorID: actorID, Name: body.Name, Color: body.Color,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": tag})
}

type updateTagBody struct {
	Name   *string `json:"name"`
	Color  *string `json:"color"`
	Status *string `json:"status"`
}

func (h *LearnerHandler) updateTag(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrLearnerTagNotFound, "标签不存在", 404))
		return
	}
	var body updateTagBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	tag, err := h.svc.UpdateTag(c.Request.Context(), brandID, actorID, id, appLearner.UpdateTagInput{
		Name: body.Name, Color: body.Color, Status: body.Status,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, tag)
}
