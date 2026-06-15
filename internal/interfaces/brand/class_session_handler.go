package brand

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appSession "github.com/zkw/mini-schedule/backend/internal/application/classsession"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// ClassSessionHandler brand 端课程场次接口。
type ClassSessionHandler struct {
	svc *appSession.Service
}

// NewClassSessionHandler 创建 handler。
func NewClassSessionHandler(svc *appSession.Service) *ClassSessionHandler {
	return &ClassSessionHandler{svc: svc}
}

// RegisterRoutes 注册 4 个 endpoint。
func (h *ClassSessionHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/class-sessions", h.list)
	g.GET("/class-sessions/:id", h.get)
	g.POST("/class-sessions", h.create)
	g.PATCH("/class-sessions/:id/cancel", h.cancel)
}

func parseOptionalTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	tu := t.UTC()
	return &tu
}

func (h *ClassSessionHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	locationID, _ := strconv.ParseInt(c.DefaultQuery("location_id", "0"), 10, 64)
	courseID, _ := strconv.ParseInt(c.DefaultQuery("course_id", "0"), 10, 64)
	instructorID, _ := strconv.ParseInt(c.DefaultQuery("instructor_profile_id", "0"), 10, 64)

	items, total, err := h.svc.List(c.Request.Context(), appSession.ListInput{
		BrandID:             brandID,
		ActorID:             actorID,
		LocationID:          locationID,
		CourseID:            courseID,
		InstructorProfileID: instructorID,
		Status:              c.Query("status"),
		From:                parseOptionalTime(c.Query("from")),
		To:                  parseOptionalTime(c.Query("to")),
		Page:                page,
		PageSize:            pageSize,
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

func (h *ClassSessionHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseSessionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	sess, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, sess)
}

type createSessionBody struct {
	CourseID            int64  `json:"course_id"`
	LocationID          int64  `json:"location_id"`
	InstructorProfileID int64  `json:"instructor_profile_id"`
	StartsAt            string `json:"starts_at"`
	EndsAt              string `json:"ends_at"`
	Capacity            int    `json:"capacity"`
	WaitlistLimit       int    `json:"waitlist_limit"`
}

func (h *ClassSessionHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createSessionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	starts, errS := time.Parse(time.RFC3339, body.StartsAt)
	ends, errE := time.Parse(time.RFC3339, body.EndsAt)
	if errS != nil || errE != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrSessionTimeInvalid, "场次时间格式错误（需 RFC3339）", 400))
		return
	}
	sess, err := h.svc.Create(c.Request.Context(), appSession.CreateInput{
		BrandID:             brandID,
		ActorID:             actorID,
		CourseID:            body.CourseID,
		LocationID:          body.LocationID,
		InstructorProfileID: body.InstructorProfileID,
		StartsAt:            starts.UTC(),
		EndsAt:              ends.UTC(),
		Capacity:            body.Capacity,
		WaitlistLimit:       body.WaitlistLimit,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": sess})
}

func (h *ClassSessionHandler) cancel(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseSessionID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		CancelReason string `json:"cancel_reason"`
	}
	_ = c.ShouldBindJSON(&body) // reason 可选
	sess, err := h.svc.Cancel(c.Request.Context(), brandID, actorID, id, body.CancelReason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, sess)
}

func parseSessionID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404)
	}
	return id, nil
}
