package brand

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// 确认 /bookings/waitlist 静态段与 /bookings/:id 参数段在 gin 路由树共存不 panic。
func TestWaitlistRoutesNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/brand")
	(&BookingHandler{}).RegisterRoutes(g)
	(&WaitlistHandler{}).RegisterRoutes(g)
}
