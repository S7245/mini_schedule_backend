package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appBooking "github.com/zkw/mini-schedule/backend/internal/application/booking"
	domainbooking "github.com/zkw/mini-schedule/backend/internal/domain/booking"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// BookingHandler brand 端预约下单 + 代取消 + 预约策略接口（Batch 13c）。
type BookingHandler struct {
	svc *appBooking.Service
}

// NewBookingHandler 创建 handler。
func NewBookingHandler(svc *appBooking.Service) *BookingHandler {
	return &BookingHandler{svc: svc}
}

// RegisterRoutes 注册预约 + 策略路由。
func (h *BookingHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/bookings", h.list)
	g.GET("/bookings/usable-entitlements", h.usableEntitlements)
	g.GET("/bookings/:id", h.get)
	g.POST("/bookings", h.create)
	g.POST("/bookings/:id/cancel", h.cancel)
	// Batch 13e 签到 / 履约 / 爽约（结束场次复用 /class-sessions 命名空间，逻辑在 booking 域）。
	g.POST("/bookings/:id/attend", h.attend)
	g.POST("/bookings/:id/no-show", h.noShow)
	g.POST("/class-sessions/:id/end", h.endSession)

	g.GET("/booking-policy", h.getPolicy)
	g.PUT("/booking-policy", h.upsertPolicy)
}

func bookingIDParam(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrBookingNotFound, "预约不存在", 404)
	}
	return id, nil
}

func (h *BookingHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	sessionID, _ := strconv.ParseInt(c.DefaultQuery("class_session_id", "0"), 10, 64)
	locationID, _ := strconv.ParseInt(c.DefaultQuery("location_id", "0"), 10, 64)
	learnerID, _ := strconv.ParseInt(c.DefaultQuery("brand_learner_profile_id", "0"), 10, 64)
	var fix *bool
	if v := c.Query("requires_entitlement_fix"); v != "" {
		b := v == "true" || v == "1"
		fix = &b
	}
	items, total, err := h.svc.List(c.Request.Context(), appBooking.ListInput{
		BrandID: brandID, ActorID: actorID,
		ClassSessionID: sessionID, LocationID: locationID, BrandLearnerProfileID: learnerID,
		Status: c.Query("status"), RequiresEntitlementFix: fix, Page: page, PageSize: pageSize,
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

func (h *BookingHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := bookingIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	bk, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, bk)
}

type createBookingBody struct {
	ClassSessionID        int64  `json:"class_session_id"`
	BrandLearnerProfileID int64  `json:"brand_learner_profile_id"`
	EntitlementMode       string `json:"entitlement_mode"`
	LearnerEntitlementID  *int64 `json:"learner_entitlement_id"`
	NoEntitlementReason   string `json:"no_entitlement_reason"`
}

func (h *BookingHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createBookingBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	bk, err := h.svc.Create(c.Request.Context(), appBooking.CreateInput{
		BrandID: brandID, ActorID: actorID,
		ClassSessionID: body.ClassSessionID, BrandLearnerProfileID: body.BrandLearnerProfileID,
		EntitlementMode: body.EntitlementMode, LearnerEntitlementID: body.LearnerEntitlementID,
		NoEntitlementReason: body.NoEntitlementReason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": bk})
}

func (h *BookingHandler) cancel(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := bookingIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	bk, err := h.svc.Cancel(c.Request.Context(), brandID, actorID, id, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, bk)
}

// ---- Batch 13e 签到 / 履约 / 爽约 ----

func (h *BookingHandler) attend(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := bookingIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	_ = c.ShouldBindJSON(&body)
	bk, err := h.svc.Attend(c.Request.Context(), brandID, actorID, id, body.Note)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, bk)
}

func (h *BookingHandler) noShow(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := bookingIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	bk, err := h.svc.ConfirmNoShow(c.Request.Context(), brandID, actorID, id, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, bk)
}

func (h *BookingHandler) endSession(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, apperr.NewAppError(apperr.ErrSessionNotFound, "场次不存在", 404))
		return
	}
	res, err := h.svc.EndSession(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, res)
}

func (h *BookingHandler) usableEntitlements(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	sessionID, _ := strconv.ParseInt(c.Query("class_session_id"), 10, 64)
	learnerID, _ := strconv.ParseInt(c.Query("brand_learner_profile_id"), 10, 64)
	if sessionID <= 0 || learnerID <= 0 {
		response.Error(c, apperr.NewAppError(apperr.ErrInvalidParam, "场次和学员不能为空", 400))
		return
	}
	items, err := h.svc.UsableEntitlements(c.Request.Context(), brandID, actorID, sessionID, learnerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

// ---- 预约策略 ----

type policyBody struct {
	BookAheadMinMinutes       int   `json:"book_ahead_min_minutes"`
	BookAheadMaxMinutes       *int  `json:"book_ahead_max_minutes"`
	CancelDeadlineMinutes     int   `json:"cancel_deadline_minutes"`
	ReleaseOnCancel           *bool `json:"release_on_cancel"`
	NoShowConsumesEntitlement *bool `json:"no_show_consumes_entitlement"`
	DailyBookingLimit         *int  `json:"daily_booking_limit"`
	WeeklyBookingLimit        *int  `json:"weekly_booking_limit"`
	ConcurrentBookingLimit    *int  `json:"concurrent_booking_limit"`
	AllowWaitlist             *bool `json:"allow_waitlist"`
	WaitlistLimit             int   `json:"waitlist_limit"`
}

func (h *BookingHandler) getPolicy(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	p, err := h.svc.GetPolicy(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func (h *BookingHandler) upsertPolicy(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body policyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.UpsertPolicy(c.Request.Context(), brandID, actorID, domainbooking.Policy{
		BookAheadMinMinutes:       body.BookAheadMinMinutes,
		BookAheadMaxMinutes:       body.BookAheadMaxMinutes,
		CancelDeadlineMinutes:     body.CancelDeadlineMinutes,
		ReleaseOnCancel:           boolOr(body.ReleaseOnCancel, true),
		NoShowConsumesEntitlement: boolOr(body.NoShowConsumesEntitlement, false),
		DailyBookingLimit:         body.DailyBookingLimit,
		WeeklyBookingLimit:        body.WeeklyBookingLimit,
		ConcurrentBookingLimit:    body.ConcurrentBookingLimit,
		AllowWaitlist:             boolOr(body.AllowWaitlist, true),
		WaitlistLimit:             body.WaitlistLimit,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}
