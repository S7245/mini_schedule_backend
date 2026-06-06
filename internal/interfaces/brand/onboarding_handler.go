package brand

import (
	"strings"

	"github.com/gin-gonic/gin"

	appOnboarding "github.com/zkw/mini-schedule/backend/internal/application/onboarding"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// OnboardingHandler brand 端 onboarding 接口。
type OnboardingHandler struct {
	svc *appOnboarding.Service
}

// NewOnboardingHandler 创建 handler。
func NewOnboardingHandler(svc *appOnboarding.Service) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

// RegisterRoutes 在已挂 JWT 的 group 上注册路由。
func (h *OnboardingHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/onboarding/status", h.getStatus)
	g.PATCH("/onboarding/steps/:step_key/skip", h.skipStep)
	g.POST("/onboarding/complete", h.complete)
}

func (h *OnboardingHandler) getStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	st, err := h.svc.GetOnboardingStatus(c.Request.Context(), brandID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, st)
}

type skipStepBody struct {
	Reason string `json:"reason"`
}

func (h *OnboardingHandler) skipStep(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	stepKey := strings.TrimSpace(c.Param("step_key"))

	var body skipStepBody
	// body 可选（reason 是可选字段）：空 body 允许；但若客户端发了非空且解析失败的 body
	// 不能静默丢弃，否则审计字段会无声为空。
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			response.Error(c, response.ErrInvalidRequest("请求体格式错误"))
			return
		}
	}

	rec, err := h.svc.SkipStep(c.Request.Context(), brandID, stepKey, body.Reason)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{
		"step_key":   rec.StepKey,
		"status":     rec.Status,
		"skipped_at": rec.SkippedAt,
	})
}

func (h *OnboardingHandler) complete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	st, err := h.svc.Complete(c.Request.Context(), brandID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{
		"overall_status":          st.OverallStatus,
		"onboarding_completed_at": st.OnboardingCompletedAt,
	})
}
