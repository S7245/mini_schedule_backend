package brand

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	appEnt "github.com/zkw/mini-schedule/backend/internal/application/entitlement"
	domainent "github.com/zkw/mini-schedule/backend/internal/domain/entitlement"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// EntitlementHandler brand 端权益产品 + 学员权益接口（Batch 13b）。
type EntitlementHandler struct {
	svc *appEnt.Service
}

// NewEntitlementHandler 创建 handler。
func NewEntitlementHandler(svc *appEnt.Service) *EntitlementHandler {
	return &EntitlementHandler{svc: svc}
}

// RegisterRoutes 注册产品 + 学员权益路由。
func (h *EntitlementHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/entitlement-products", h.listProducts)
	g.GET("/entitlement-products/:id", h.getProduct)
	g.POST("/entitlement-products", h.createProduct)
	g.PATCH("/entitlement-products/:id", h.updateProduct)
	g.PATCH("/entitlement-products/:id/status", h.updateProductStatus)

	g.GET("/learners/:id/entitlements", h.listLearnerEntitlements)
	g.POST("/learners/:id/entitlements", h.grant)
	g.GET("/entitlements/:id/transactions", h.listTransactions)
	g.POST("/entitlements/:id/adjust", h.adjust)
	g.PATCH("/entitlements/:id/status", h.setEntitlementStatus)
}

func entIDParam(c *gin.Context, code apperr.ErrorCode, msg string) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return 0, apperr.NewAppError(code, msg, 404)
	}
	return id, nil
}

// ---- 产品 ----

func (h *EntitlementHandler) listProducts(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	items, total, err := h.svc.ListProducts(c.Request.Context(), brandID, actorID, c.Query("status"), c.Query("product_type"), page, pageSize)
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

func (h *EntitlementHandler) getProduct(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementProductNotFound, "权益产品不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	p, err := h.svc.GetProduct(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

type productBody struct {
	Name                   string  `json:"name"`
	Description            string  `json:"description"`
	ProductType            string  `json:"product_type"`
	TotalCredits           int     `json:"total_credits"`
	ValidityDays           int     `json:"validity_days"`
	DailyBookingLimit      int     `json:"daily_booking_limit"`
	WeeklyBookingLimit     int     `json:"weekly_booking_limit"`
	MonthlyBookingLimit    int     `json:"monthly_booking_limit"`
	ConcurrentBookingLimit int     `json:"concurrent_booking_limit"`
	LocationScope          string  `json:"location_scope"`
	CourseScope            string  `json:"course_scope"`
	LocationIDs            []int64 `json:"location_ids"`
	CourseIDs              []int64 `json:"course_ids"`
}

func (h *EntitlementHandler) createProduct(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body productBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.CreateProduct(c.Request.Context(), domainent.CreateProductInput{
		BrandID:                brandID,
		ActorID:                actorID,
		Name:                   body.Name,
		Description:            body.Description,
		ProductType:            body.ProductType,
		TotalCredits:           body.TotalCredits,
		ValidityDays:           body.ValidityDays,
		DailyBookingLimit:      body.DailyBookingLimit,
		WeeklyBookingLimit:     body.WeeklyBookingLimit,
		MonthlyBookingLimit:    body.MonthlyBookingLimit,
		ConcurrentBookingLimit: body.ConcurrentBookingLimit,
		LocationScope:          body.LocationScope,
		CourseScope:            body.CourseScope,
		LocationIDs:            body.LocationIDs,
		CourseIDs:              body.CourseIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": p})
}

type updateProductBody struct {
	Name                   *string  `json:"name"`
	Description            *string  `json:"description"`
	TotalCredits           *int     `json:"total_credits"`
	ValidityDays           *int     `json:"validity_days"`
	DailyBookingLimit      *int     `json:"daily_booking_limit"`
	WeeklyBookingLimit     *int     `json:"weekly_booking_limit"`
	MonthlyBookingLimit    *int     `json:"monthly_booking_limit"`
	ConcurrentBookingLimit *int     `json:"concurrent_booking_limit"`
	LocationScope          *string  `json:"location_scope"`
	CourseScope            *string  `json:"course_scope"`
	LocationIDs            *[]int64 `json:"location_ids"`
	CourseIDs              *[]int64 `json:"course_ids"`
}

func (h *EntitlementHandler) updateProduct(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementProductNotFound, "权益产品不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateProductBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	p, err := h.svc.UpdateProduct(c.Request.Context(), brandID, actorID, id, domainent.UpdateProductInput{
		Name:                   body.Name,
		Description:            body.Description,
		TotalCredits:           body.TotalCredits,
		ValidityDays:           body.ValidityDays,
		DailyBookingLimit:      body.DailyBookingLimit,
		WeeklyBookingLimit:     body.WeeklyBookingLimit,
		MonthlyBookingLimit:    body.MonthlyBookingLimit,
		ConcurrentBookingLimit: body.ConcurrentBookingLimit,
		LocationScope:          body.LocationScope,
		CourseScope:            body.CourseScope,
		LocationIDs:            body.LocationIDs,
		CourseIDs:              body.CourseIDs,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

func (h *EntitlementHandler) updateProductStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementProductNotFound, "权益产品不存在")
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
	p, err := h.svc.UpdateProductStatus(c.Request.Context(), brandID, actorID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, p)
}

// ---- 学员权益 ----

func (h *EntitlementHandler) listLearnerEntitlements(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	learnerID, err := entIDParam(c, apperr.ErrLearnerNotFound, "学员不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	items, err := h.svc.ListByLearner(c.Request.Context(), brandID, actorID, learnerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

type grantBody struct {
	ProductID int64  `json:"product_id"`
	StartsAt  string `json:"starts_at"`
	Remark    string `json:"remark"`
}

func (h *EntitlementHandler) grant(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	learnerID, err := entIDParam(c, apperr.ErrLearnerNotFound, "学员不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	var body grantBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	var startsAt *time.Time
	if s := body.StartsAt; s != "" {
		ts, perr := time.Parse(time.RFC3339, s)
		if perr != nil {
			response.Error(c, apperr.NewAppError(apperr.ErrInvalidParam, "生效日期格式错误", 400))
			return
		}
		startsAt = &ts
	}
	e, err := h.svc.Grant(c.Request.Context(), domainent.GrantInput{
		BrandID: brandID, ActorID: actorID, LearnerID: learnerID, ProductID: body.ProductID, StartsAt: startsAt, Remark: body.Remark,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": e})
}

func (h *EntitlementHandler) listTransactions(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementNotFound, "权益不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	items, err := h.svc.ListTransactions(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, items)
}

func (h *EntitlementHandler) adjust(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementNotFound, "权益不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Delta  int    `json:"delta"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	e, err := h.svc.Adjust(c.Request.Context(), domainent.AdjustInput{
		BrandID: brandID, ActorID: actorID, EntitlementID: id, Delta: body.Delta, Reason: body.Reason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, e)
}

func (h *EntitlementHandler) setEntitlementStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := entIDParam(c, apperr.ErrEntitlementNotFound, "权益不存在")
	if err != nil {
		response.Error(c, err)
		return
	}
	var body struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	e, err := h.svc.SetStatus(c.Request.Context(), brandID, actorID, id, body.Status, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, e)
}
