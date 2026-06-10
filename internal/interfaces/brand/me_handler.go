package brand

import (
	"github.com/gin-gonic/gin"

	"github.com/zkw/mini-schedule/backend/internal/application/rbac"
	domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// MeHandler 暴露当前登录用户的自服务接口。
//
// Batch 6 T08：仅 GET /me/permissions，给前端 usePermissions hook 拿权限集合 + data_scope。
type MeHandler struct {
	checker *rbac.Checker
}

// NewMeHandler 创建。checker 不可为 nil。
func NewMeHandler(checker *rbac.Checker) *MeHandler {
	return &MeHandler{checker: checker}
}

// RegisterRoutes 在 /api/v1/brand 下挂 /me/*。
func (h *MeHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/me/permissions", h.permissions)
}

type mePermissionsResponse struct {
	Permissions []string         `json:"permissions"`
	DataScope   meDataScopeBlock `json:"data_scope"`
}

type meDataScopeBlock struct {
	Kind        string  `json:"kind"`
	LocationIDs []int64 `json:"location_ids,omitempty"`
}

func (h *MeHandler) permissions(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)

	perms, scope, err := h.checker.Resolve(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 序列化 PermissionSet → []string；DataScope 仅当 AssignedLocations 才暴露 location_ids
	codes := perms.Codes()
	body := mePermissionsResponse{
		Permissions: codes,
		DataScope: meDataScopeBlock{
			Kind: string(scope.Kind),
		},
	}
	if scope.Kind == domainrbac.DataScopeAssignedLocations {
		body.DataScope.LocationIDs = scope.LocationIDs
	}

	response.Success(c, body)
}
