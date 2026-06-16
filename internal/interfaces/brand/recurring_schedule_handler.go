package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appRecurring "github.com/zkw/mini-schedule/backend/internal/application/recurringschedule"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// RecurringScheduleHandler brand 端循环排课接口。
type RecurringScheduleHandler struct {
	svc *appRecurring.Service
}

// NewRecurringScheduleHandler 创建 handler。
func NewRecurringScheduleHandler(svc *appRecurring.Service) *RecurringScheduleHandler {
	return &RecurringScheduleHandler{svc: svc}
}

// RegisterRoutes 注册 4 个 endpoint。
func (h *RecurringScheduleHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/recurring-schedules", h.list)
	g.GET("/recurring-schedules/:id", h.get)
	g.POST("/recurring-schedules", h.generate)
	g.PATCH("/recurring-schedules/:id/cancel", h.cancel)
}

func parseRecurringID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrRecurringNotFound, "循环排课不存在", 404)
	}
	return id, nil
}

func (h *RecurringScheduleHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	locationID, _ := strconv.ParseInt(c.DefaultQuery("location_id", "0"), 10, 64)

	items, total, err := h.svc.List(c.Request.Context(), appRecurring.ListInput{
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

func (h *RecurringScheduleHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseRecurringID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	sch, sessions, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"recurring_schedule": sch, "sessions": sessions})
}

type generateRecurringBody struct {
	CourseID            int64  `json:"course_id"`
	LocationID          int64  `json:"location_id"`
	LocationResourceID  *int64 `json:"location_resource_id"`
	InstructorProfileID int64  `json:"instructor_profile_id"`
	Weekdays            []int  `json:"weekdays"`
	StartDate           string `json:"start_date"`
	EndDate             string `json:"end_date"`
	RepeatWeeks         *int   `json:"repeat_weeks"`
	StartTime           string `json:"start_time"`
	DurationMin         int    `json:"duration_min"`
	Capacity            int    `json:"capacity"`
}

func (h *RecurringScheduleHandler) generate(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body generateRecurringBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	result, err := h.svc.Generate(c.Request.Context(), appRecurring.GenerateInput{
		BrandID:             brandID,
		ActorID:             actorID,
		CourseID:            body.CourseID,
		LocationID:          body.LocationID,
		LocationResourceID:  body.LocationResourceID,
		InstructorProfileID: body.InstructorProfileID,
		Weekdays:            body.Weekdays,
		StartDate:           body.StartDate,
		EndDate:             body.EndDate,
		RepeatWeeks:         body.RepeatWeeks,
		StartTime:           body.StartTime,
		DurationMin:         body.DurationMin,
		Capacity:            body.Capacity,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"code":    "OK",
		"message": "created",
		"data": gin.H{
			"recurring_schedule": result.Schedule,
			"created_count":      len(result.Created),
			"skipped_count":      len(result.Skipped),
			"created":            result.Created,
			"skipped":            result.Skipped,
		},
	})
}

func (h *RecurringScheduleHandler) cancel(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseRecurringID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	sch, err := h.svc.Cancel(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, sch)
}
