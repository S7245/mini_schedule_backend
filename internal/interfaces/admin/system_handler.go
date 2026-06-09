package admin

import (
	"github.com/gin-gonic/gin"

	appStaff "github.com/zkw/mini-schedule/backend/internal/application/staff"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// SystemHandler 平台运维杂项接口（Batch 5 起放 backfill / 校验工具）。
type SystemHandler struct {
	allocator *appStaff.RoleAllocator
}

func NewSystemHandler(allocator *appStaff.RoleAllocator) *SystemHandler {
	return &SystemHandler{allocator: allocator}
}

func (h *SystemHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.POST("/system/backfill-owner-roles", h.backfillOwnerRoles)
}

// backfillOwnerRoles 遍历所有 is_owner=true 的 brand_user 调 AssignDefaultOwnerRoles。
// 幂等：第二次跑应该 skipped=N。
func (h *SystemHandler) backfillOwnerRoles(c *gin.Context) {
	actorID := middleware.GetUserID(c)
	res, err := h.allocator.BackfillOwnerRoles(c.Request.Context(), actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, res)
}
