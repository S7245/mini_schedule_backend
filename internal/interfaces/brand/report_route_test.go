package brand

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestReportRouteRegistered 确认 GET /reports/overview 注册进 gin 路由树。
func TestReportRouteRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/brand")
	(&ReportHandler{}).RegisterRoutes(g)

	for _, ri := range r.Routes() {
		if ri.Method == "GET" && ri.Path == "/api/v1/brand/reports/overview" {
			return
		}
	}
	t.Fatal("GET /api/v1/brand/reports/overview not registered")
}
