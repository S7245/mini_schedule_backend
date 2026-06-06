package admin

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	commercialapp "github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

type saasPlanFeatureRequest struct {
	FeatureCode string `json:"feature_code" validate:"required"`
	Enabled     bool   `json:"enabled"`
}

type createSaaSPlanRequest struct {
	Name              string                   `json:"name" validate:"required,min=2,max=100"`
	Description       string                   `json:"description" validate:"omitempty,max=1000"`
	MonthlyPrice      string                   `json:"monthly_price" validate:"required"`
	YearlyPrice       string                   `json:"yearly_price" validate:"required"`
	YearlyDiscountPct *string                  `json:"yearly_discount_pct"`
	Currency          string                   `json:"currency" validate:"omitempty,len=3"`
	MaxLocations      int                      `json:"max_locations" validate:"required,gt=0"`
	MaxStaffSeats     int                      `json:"max_staff_seats" validate:"required,gt=0"`
	MaxLearners       int                      `json:"max_learners" validate:"required,gt=0"`
	SortOrder         int                      `json:"sort_order"`
	Features          []saasPlanFeatureRequest `json:"features"`
}

type updateSaaSPlanRequest struct {
	Name              *string                   `json:"name" validate:"omitempty,min=2,max=100"`
	Description       *string                   `json:"description" validate:"omitempty,max=1000"`
	MonthlyPrice      *string                   `json:"monthly_price"`
	YearlyPrice       *string                   `json:"yearly_price"`
	YearlyDiscountPct *string                   `json:"yearly_discount_pct"`
	Currency          *string                   `json:"currency" validate:"omitempty,len=3"`
	MaxLocations      *int                      `json:"max_locations" validate:"omitempty,gt=0"`
	MaxStaffSeats     *int                      `json:"max_staff_seats" validate:"omitempty,gt=0"`
	MaxLearners       *int                      `json:"max_learners" validate:"omitempty,gt=0"`
	SortOrder         *int                      `json:"sort_order"`
	Features          *[]saasPlanFeatureRequest `json:"features"`
}

type manualRenewBrandSubscriptionRequest struct {
	ExtendMonths int    `json:"extend_months" validate:"omitempty,gte=0"`
	ExtendDays   int    `json:"extend_days" validate:"omitempty,gte=0"`
	Reason       string `json:"reason" validate:"required,max=1000"`
}

type updateBrandSubscriptionLimitsRequest struct {
	MaxLocations  *int                      `json:"max_locations" validate:"omitempty,gt=0"`
	MaxStaffSeats *int                      `json:"max_staff_seats" validate:"omitempty,gt=0"`
	MaxLearners   *int                      `json:"max_learners" validate:"omitempty,gt=0"`
	Features      *[]saasPlanFeatureRequest `json:"features"`
	Reason        string                    `json:"reason" validate:"required,max=1000"`
}

type updateBrandSubscriptionStatusRequest struct {
	Status       string `json:"status" validate:"required,oneof=active grace_period restricted frozen expired cancelled"`
	FrozenReason string `json:"frozen_reason" validate:"omitempty,max=500"`
	Reason       string `json:"reason" validate:"required,max=1000"`
}

type createPublicSignupOrderRequest struct {
	Phone          string `json:"phone" validate:"required"`
	SMSCode        string `json:"sms_code" validate:"required"`
	Password       string `json:"password" validate:"required,min=6,max=64"`
	BrandName      string `json:"brand_name" validate:"required,min=1,max=100"`
	LogoURL        string `json:"logo_url" validate:"omitempty,url"`
	ContactName    string `json:"contact_name" validate:"required,min=1,max=50"`
	ContactEmail   string `json:"contact_email" validate:"omitempty,email,max=100"`
	IndustryType   string `json:"industry_type" validate:"omitempty,max=50"`
	PlanID         int64  `json:"plan_id" validate:"required,gt=0"`
	BillingCycle   string `json:"billing_cycle" validate:"required,oneof=monthly yearly"`
	PaymentChannel string `json:"payment_channel" validate:"omitempty,oneof=wechat alipay"`
}

func (h *Handler) RegisterPublicRoutes(r *gin.RouterGroup) {
	r.GET("/saas-plans", h.listPublicSaaSPlans)
	r.POST("/signup/sms-code", h.requestSignupSMSCode)
	r.POST("/signup/pre-validate", h.preValidateSignup)
	r.POST("/signup/orders", h.createPublicSignupOrder)
	r.POST("/payment/native", h.createWeChatNativePay)
	r.GET("/payment/orders/:order_id", h.getOrderPaymentStatus)
	r.POST("/payment/callback", h.handleWeChatPaymentCallback)
}

// handleWeChatPaymentCallback 处理微信支付 v3 异步回调。
//
// 响应规则：
//   - 验签 / 时间戳失败 → 401 + {code:"UNAUTHORIZED", message:"invalid signature"}
//     （微信会按 401 重试，对真实环境是合理的；mock 模式下也方便测试断言）
//   - 其他业务情况（订单不存在、金额不一致、trade_state 非 SUCCESS、幂等成功 …）
//     全部按 200 success 返回，避免微信无意义重试；记录由 service / repo 落到
//     PaymentCallbackLog / OperationLog。
//   - 仅在真正的内部错误（如数据库不可用）才返回 500，让微信重试。
func (h *Handler) handleWeChatPaymentCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "INVALID_REQUEST", "message": "failed to read body"})
		return
	}

	headers := make(map[string]string, len(c.Request.Header))
	for k, v := range c.Request.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	result, err := h.commercialSvc.ProcessWeChatPaymentCallback(c.Request.Context(), commercialapp.ProcessWeChatPaymentCallbackInput{
		Headers: headers,
		Body:    body,
	})
	if err != nil {
		// 验签 / 解密 / timestamp 失败 → AppError(401)；其余 → 500
		if appErr := apperr.GetAppError(err); appErr != nil && appErr.HTTPStatus == http.StatusUnauthorized {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    string(apperr.ErrUnauthorized),
				"message": "invalid signature",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    string(apperr.ErrInternalServer),
			"message": "internal error",
		})
		return
	}

	_ = result // 当前响应体不暴露 result 细节，避免泄漏内部 ID
	c.JSON(http.StatusOK, gin.H{
		"code":    "OK",
		"message": "success",
	})
}

func (h *Handler) registerCommercialRoutes(r *gin.RouterGroup) {
	r.GET("/platform/summary", h.getPlatformSummary)

	r.GET("/saas-plans", h.listSaaSPlans)
	r.POST("/saas-plans", h.createSaaSPlan)
	r.GET("/saas-plans/:id", h.getSaaSPlan)
	r.PUT("/saas-plans/:id", h.updateSaaSPlan)
	r.PATCH("/saas-plans/:id/status", h.updateSaaSPlanStatus)

	r.GET("/saas-plan-orders", h.listSaaSPlanOrders)
	r.GET("/brand-subscriptions", h.listBrandSubscriptions)
	r.POST("/brand-subscriptions/:id/renew", h.manualRenewBrandSubscription)
	r.PATCH("/brand-subscriptions/:id/limits", h.updateBrandSubscriptionLimits)
	r.PATCH("/brand-subscriptions/:id/status", h.updateBrandSubscriptionStatus)
	r.GET("/payment-transactions", h.listPaymentTransactions)
	r.GET("/payment-callback-logs", h.listPaymentCallbackLogs)
	r.GET("/operation-logs", h.listOperationLogs)
}

func (h *Handler) listPublicSaaSPlans(c *gin.Context) {
	result, err := h.commercialSvc.ListPublicSaaSPlans(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) requestSignupSMSCode(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" validate:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	if err := h.commercialSvc.RequestSignupSMSCode(c.Request.Context(), req.Phone, c.ClientIP()); err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessNoData(c)
}

func (h *Handler) preValidateSignup(c *gin.Context) {
	var req struct {
		Phone    string `json:"phone" validate:"required"`
		SMSCode  string `json:"sms_code" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	if err := h.commercialSvc.PreValidateSignup(c.Request.Context(), req.Phone, req.SMSCode, req.Password); err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessNoData(c)
}

func (h *Handler) createPublicSignupOrder(c *gin.Context) {
	var req createPublicSignupOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}

	result, err := h.commercialSvc.CreatePublicSignupOrder(c.Request.Context(), commercial.CreatePublicSignupOrderInput{
		Phone:          req.Phone,
		SMSCode:        req.SMSCode,
		Password:       req.Password,
		BrandName:      req.BrandName,
		LogoURL:        req.LogoURL,
		ContactName:    req.ContactName,
		ContactEmail:   req.ContactEmail,
		IndustryType:   req.IndustryType,
		PlanID:         req.PlanID,
		BillingCycle:   commercial.BillingCycle(req.BillingCycle),
		PaymentChannel: commercial.PaymentChannel(req.PaymentChannel),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) createWeChatNativePay(c *gin.Context) {
	var req struct {
		OrderID int64 `json:"order_id" validate:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.commercialSvc.CreateWeChatNativePay(c.Request.Context(), req.OrderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) getOrderPaymentStatus(c *gin.Context) {
	orderID, err := strconv.ParseInt(c.Param("order_id"), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的订单 ID"))
		return
	}
	result, err := h.commercialSvc.GetOrderPaymentStatus(c.Request.Context(), orderID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) listSaaSPlans(c *gin.Context) {
	page, pageSize := getPageParams(c)
	includeInactive := c.Query("include_inactive") == "true"
	items, total, err := h.commercialSvc.ListSaaSPlans(c.Request.Context(), page, pageSize, includeInactive)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) createSaaSPlan(c *gin.Context) {
	var req createSaaSPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.commercialSvc.CreateSaaSPlan(c.Request.Context(), commercial.CreateSaaSPlanInput{
		Name:              req.Name,
		Description:       req.Description,
		MonthlyPrice:      req.MonthlyPrice,
		YearlyPrice:       req.YearlyPrice,
		YearlyDiscountPct: req.YearlyDiscountPct,
		Currency:          req.Currency,
		MaxLocations:      req.MaxLocations,
		MaxStaffSeats:     req.MaxStaffSeats,
		MaxLearners:       req.MaxLearners,
		SortOrder:         req.SortOrder,
		Features:          toFeatureInputs(req.Features),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) getSaaSPlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	result, err := h.commercialSvc.GetSaaSPlan(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) updateSaaSPlan(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req updateSaaSPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	var features *[]commercial.SaaSPlanFeatureInput
	if req.Features != nil {
		values := toFeatureInputs(*req.Features)
		features = &values
	}
	result, err := h.commercialSvc.UpdateSaaSPlan(c.Request.Context(), id, commercial.UpdateSaaSPlanInput{
		Name:              req.Name,
		Description:       req.Description,
		MonthlyPrice:      req.MonthlyPrice,
		YearlyPrice:       req.YearlyPrice,
		YearlyDiscountPct: req.YearlyDiscountPct,
		Currency:          req.Currency,
		MaxLocations:      req.MaxLocations,
		MaxStaffSeats:     req.MaxStaffSeats,
		MaxLearners:       req.MaxLearners,
		SortOrder:         req.SortOrder,
		Features:          features,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) updateSaaSPlanStatus(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status" validate:"required,oneof=active inactive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	if err := h.commercialSvc.UpdateSaaSPlanStatus(c.Request.Context(), id, commercial.SaaSPlanStatus(req.Status)); err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessNoData(c)
}

func (h *Handler) listSaaSPlanOrders(c *gin.Context) {
	page, pageSize := getPageParams(c)
	brandID, _ := strconv.ParseInt(c.Query("brand_id"), 10, 64)
	items, total, err := h.commercialSvc.ListSaaSPlanOrders(c.Request.Context(), page, pageSize, commercial.ListSaaSPlanOrdersFilter{
		Status:         commercial.SaaSPlanOrderStatus(c.Query("status")),
		PaymentChannel: commercial.PaymentChannel(c.Query("payment_channel")),
		Source:         commercial.OrderSource(c.Query("source")),
		BrandID:        brandID,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) listBrandSubscriptions(c *gin.Context) {
	page, pageSize := getPageParams(c)
	brandID, _ := strconv.ParseInt(c.Query("brand_id"), 10, 64)
	items, total, err := h.commercialSvc.ListBrandSubscriptions(c.Request.Context(), page, pageSize, commercial.ListBrandSubscriptionsFilter{
		Status:  commercial.BrandSubscriptionStatus(c.Query("status")),
		BrandID: brandID,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) manualRenewBrandSubscription(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req manualRenewBrandSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.commercialSvc.ManualRenewBrandSubscription(c.Request.Context(), id, commercial.ManualRenewBrandSubscriptionInput{
		ActorID:      middleware.GetUserID(c),
		ExtendMonths: req.ExtendMonths,
		ExtendDays:   req.ExtendDays,
		Reason:       req.Reason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) updateBrandSubscriptionLimits(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req updateBrandSubscriptionLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	var features *[]commercial.SaaSPlanFeatureInput
	if req.Features != nil {
		values := toFeatureInputs(*req.Features)
		features = &values
	}
	result, err := h.commercialSvc.UpdateBrandSubscriptionLimits(c.Request.Context(), id, commercial.UpdateBrandSubscriptionLimitsInput{
		ActorID:       middleware.GetUserID(c),
		MaxLocations:  req.MaxLocations,
		MaxStaffSeats: req.MaxStaffSeats,
		MaxLearners:   req.MaxLearners,
		Features:      features,
		Reason:        req.Reason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) updateBrandSubscriptionStatus(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req updateBrandSubscriptionStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if err := h.validator.Struct(req); err != nil {
		response.Error(c, h.validator.InvalidRequest(c, err))
		return
	}
	result, err := h.commercialSvc.UpdateBrandSubscriptionStatus(c.Request.Context(), id, commercial.UpdateBrandSubscriptionStatusInput{
		ActorID:      middleware.GetUserID(c),
		Status:       commercial.BrandSubscriptionStatus(req.Status),
		FrozenReason: req.FrozenReason,
		Reason:       req.Reason,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func (h *Handler) listPaymentTransactions(c *gin.Context) {
	page, pageSize := getPageParams(c)
	brandID, _ := strconv.ParseInt(c.Query("brand_id"), 10, 64)
	orderID, _ := strconv.ParseInt(c.Query("order_id"), 10, 64)
	items, total, err := h.commercialSvc.ListPaymentTransactions(c.Request.Context(), page, pageSize, commercial.ListPaymentTransactionsFilter{
		Status:         commercial.PaymentTransactionStatus(c.Query("status")),
		PaymentChannel: commercial.PaymentChannel(c.Query("payment_channel")),
		OrderID:        orderID,
		BrandID:        brandID,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) listPaymentCallbackLogs(c *gin.Context) {
	page, pageSize := getPageParams(c)
	items, total, err := h.commercialSvc.ListPaymentCallbackLogs(
		c.Request.Context(),
		page,
		pageSize,
		commercial.PaymentCallbackLogStatus(c.Query("status")),
	)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) listOperationLogs(c *gin.Context) {
	page, pageSize := getPageParams(c)
	brandID, _ := strconv.ParseInt(c.Query("brand_id"), 10, 64)
	targetID, _ := strconv.ParseInt(c.Query("target_id"), 10, 64)
	items, total, err := h.commercialSvc.ListOperationLogs(c.Request.Context(), page, pageSize, commercial.ListOperationLogsFilter{
		BrandID:    brandID,
		Action:     c.Query("action"),
		TargetType: c.Query("target_type"),
		TargetID:   targetID,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *Handler) getPlatformSummary(c *gin.Context) {
	result, err := h.commercialSvc.GetPlatformSummary(c.Request.Context())
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, result)
}

func getPageParams(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	return page, pageSize
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		response.Error(c, response.ErrInvalidRequest("无效的 ID"))
		return 0, false
	}
	return id, true
}

func toFeatureInputs(req []saasPlanFeatureRequest) []commercial.SaaSPlanFeatureInput {
	items := make([]commercial.SaaSPlanFeatureInput, len(req))
	for i := range req {
		items[i] = commercial.SaaSPlanFeatureInput{
			FeatureCode: req[i].FeatureCode,
			Enabled:     req[i].Enabled,
		}
	}
	return items
}
