package brand

import (
	"strconv"

	"github.com/gin-gonic/gin"

	appreport "github.com/zkw/mini-schedule/backend/internal/application/report"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// ReportHandler 品牌基础运营看板接口（Batch 17）。薄 handler：透传 query → service。
// 权限门 report.view_basic + data_scope 在 application/report.Service 内。
type ReportHandler struct {
	svc *appreport.Service
}

// NewReportHandler 创建 handler。
func NewReportHandler(svc *appreport.Service) *ReportHandler {
	return &ReportHandler{svc: svc}
}

// RegisterRoutes 注册报表路由。
func (h *ReportHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/reports/overview", h.overview)
}

// overview GET /reports/overview?range=&from=&to=&location_id=
func (h *ReportHandler) overview(c *gin.Context) {
	in := appreport.OverviewInput{
		BrandID:  middleware.GetBrandID(c),
		ActorID:  middleware.GetUserID(c),
		Preset:   c.Query("range"),
		FromDate: c.Query("from"),
		ToDate:   c.Query("to"),
	}
	if v := c.Query("location_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			response.Error(c, response.ErrInvalidRequest("无效的门店 ID"))
			return
		}
		in.LocationID = &id
	}
	out, err := h.svc.GetBrandOverview(c.Request.Context(), in)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, out)
}
