package admin

// 本文件仅用于 swag 注解生成 Swagger 文档，实际路由注册在 handler.go 中。

// @Summary 管理员登录
// @Description 使用用户名和密码登录管理后台
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body LoginRequest true "登录信息"
// @Success 200 {object} response.Response{data=LoginResponse} "成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "认证失败"
// @Router /api/v1/admin/login [post]
func _swaggerLogin() {}

// @Summary 创建品牌
// @Description 创建新品牌入驻
// @Tags 品牌管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateBrandRequest true "品牌信息"
// @Success 200 {object} response.Response "成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/admin/brands [post]
func _swaggerCreateBrand() {}

// @Summary 品牌列表
// @Description 分页获取品牌列表
// @Tags 品牌管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response "成功"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/admin/brands [get]
func _swaggerListBrands() {}

// @Summary 品牌详情
// @Description 获取单个品牌详情
// @Tags 品牌管理
// @Produce json
// @Security BearerAuth
// @Param id path int true "品牌 ID"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "品牌不存在"
// @Router /api/v1/admin/brands/{id} [get]
func _swaggerGetBrand() {}

// @Summary 更新品牌
// @Description 更新品牌信息
// @Tags 品牌管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "品牌 ID"
// @Param body body UpdateBrandRequest true "品牌信息"
// @Success 200 {object} response.Response "成功"
// @Router /api/v1/admin/brands/{id} [put]
func _swaggerUpdateBrand() {}

// @Summary 更新品牌状态
// @Description 启用/禁用/待审核品牌
// @Tags 品牌管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "品牌 ID"
// @Param body body UpdateBrandStatusRequest true "状态"
// @Success 200 {object} response.Response "成功"
// @Router /api/v1/admin/brands/{id}/status [patch]
func _swaggerUpdateBrandStatus() {}

// UpdateBrandStatusRequest 更新品牌状态请求体（仅用于 swag 文档）
type UpdateBrandStatusRequest struct {
	Status string `json:"status" example:"active"`
}

// @Summary 创建管理员
// @Description 创建平台管理员账号
// @Tags 管理员管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateAdminRequest true "管理员信息"
// @Success 200 {object} response.Response "成功"
// @Router /api/v1/admin/admins [post]
func _swaggerCreateAdmin() {}

// @Summary 管理员列表
// @Description 分页获取管理员列表
// @Tags 管理员管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response "成功"
// @Router /api/v1/admin/admins [get]
func _swaggerListAdmins() {}
