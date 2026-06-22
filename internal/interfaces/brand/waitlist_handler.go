package brand

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appWaitlist "github.com/zkw/mini-schedule/backend/internal/application/waitlist"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// WaitlistHandler brand 端候补接口（Batch 13d）。
type WaitlistHandler struct {
	svc *appWaitlist.Service
}

// NewWaitlistHandler 创建 handler。
func NewWaitlistHandler(svc *appWaitlist.Service) *WaitlistHandler {
	return &WaitlistHandler{svc: svc}
}

// RegisterRoutes 注册候补路由（/bookings/waitlist 静态段与 /bookings/:id 参数段共存）。
func (h *WaitlistHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/bookings/waitlist", h.list)
	g.POST("/bookings/waitlist", h.join)
	g.POST("/bookings/waitlist/:id/promote", h.promote)
	g.POST("/bookings/waitlist/:id/skip", h.skip)
	g.POST("/bookings/waitlist/:id/cancel", h.cancel)
}

func waitlistIDParam(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(apperr.ErrWaitlistEntryNotFound, "候补不存在", 404)
	}
	return id, nil
}

func (h *WaitlistHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	sessionID, _ := strconv.ParseInt(c.Query("class_session_id"), 10, 64)
	if sessionID <= 0 {
		response.Error(c, apperr.NewAppError(apperr.ErrInvalidParam, "场次不能为空", 400))
		return
	}
	items, err := h.svc.List(c.Request.Context(), brandID, actorID, sessionID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

type joinWaitlistBody struct {
	ClassSessionID        int64 `json:"class_session_id"`
	BrandLearnerProfileID int64 `json:"brand_learner_profile_id"`
}

func (h *WaitlistHandler) join(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body joinWaitlistBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	entry, err := h.svc.Join(c.Request.Context(), brandID, actorID, body.ClassSessionID, body.BrandLearnerProfileID)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": entry})
}

type promoteBody struct {
	EntitlementMode      string `json:"entitlement_mode"`
	LearnerEntitlementID *int64 `json:"learner_entitlement_id"`
	NoEntitlementReason  string `json:"no_entitlement_reason"`
}

func (h *WaitlistHandler) promote(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := waitlistIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body promoteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	entry, err := h.svc.Promote(c.Request.Context(), appWaitlist.PromoteParams{
		BrandID: brandID, ActorID: actorID, EntryID: id,
		EntitlementMode: body.EntitlementMode, LearnerEntitlementID: body.LearnerEntitlementID, NoEntitlementReason: body.NoEntitlementReason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *WaitlistHandler) skip(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := waitlistIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	entry, err := h.svc.Skip(c.Request.Context(), brandID, actorID, id, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, entry)
}

func (h *WaitlistHandler) cancel(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := waitlistIDParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	entry, err := h.svc.Cancel(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, entry)
}
