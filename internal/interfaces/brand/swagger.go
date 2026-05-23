package brand

// 本文件仅用于 swag 注解生成 Swagger 文档，实际路由注册在 handler.go 中。

// @Summary 品牌管理员登录
// @Description 使用手机号和密码登录品牌商家后台
// @Tags 认证
// @Accept json
// @Produce json
// @Param body body LoginRequest true "登录信息"
// @Success 200 {object} response.Response{data=LoginResponse} "成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "认证失败"
// @Router /api/v1/brand/login [post]
func _swaggerLogin() {}

// @Summary 创建子账号
// @Description 为当前品牌创建新的管理员子账号
// @Tags 用户管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateUserRequest true "用户信息"
// @Success 200 {object} response.Response "成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/brand/users [post]
func _swaggerCreateUser() {}

// @Summary C 端用户列表
// @Description 分页获取当前品牌下的 C 端用户列表
// @Tags 用户管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response "成功"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/brand/users [get]
func _swaggerListUsers() {}

// @Summary C 端用户详情
// @Description 获取单个 C 端用户详情
// @Tags 用户管理
// @Produce json
// @Security BearerAuth
// @Param id path int true "用户 ID"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "用户不存在"
// @Router /api/v1/brand/users/{id} [get]
func _swaggerGetUser() {}

// @Summary 创建课程
// @Description 为当前品牌创建新课程
// @Tags 课程管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body CreateCourseRequest true "课程信息"
// @Success 200 {object} response.Response "成功"
// @Failure 400 {object} response.Response "参数错误"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/brand/courses [post]
func _swaggerCreateCourse() {}

// @Summary 课程列表
// @Description 分页获取当前品牌下的课程列表
// @Tags 课程管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response "成功"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/brand/courses [get]
func _swaggerListCourses() {}

// @Summary 课程详情
// @Description 获取单个课程详情
// @Tags 课程管理
// @Produce json
// @Security BearerAuth
// @Param id path int true "课程 ID"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "课程不存在"
// @Router /api/v1/brand/courses/{id} [get]
func _swaggerGetCourse() {}

// @Summary 更新课程
// @Description 更新课程信息
// @Tags 课程管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "课程 ID"
// @Param body body CreateCourseRequest true "课程信息"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "课程不存在"
// @Router /api/v1/brand/courses/{id} [put]
func _swaggerUpdateCourse() {}

// @Summary 删除课程
// @Description 删除指定课程
// @Tags 课程管理
// @Security BearerAuth
// @Param id path int true "课程 ID"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "课程不存在"
// @Router /api/v1/brand/courses/{id} [delete]
func _swaggerDeleteCourse() {}

// @Summary 更新课程状态
// @Description 更新课程发布状态（draft/published/archived）
// @Tags 课程管理
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "课程 ID"
// @Param body body UpdateCourseStatusRequest true "状态"
// @Success 200 {object} response.Response "成功"
// @Failure 404 {object} response.Response "课程不存在"
// @Router /api/v1/brand/courses/{id}/status [patch]
func _swaggerUpdateCourseStatus() {}

// @Summary 训练记录列表
// @Description 分页获取当前品牌下的训练记录列表
// @Tags 训练管理
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} response.Response "成功"
// @Failure 401 {object} response.Response "未认证"
// @Router /api/v1/brand/trainings [get]
func _swaggerListTrainings() {}

// UpdateCourseStatusRequest 更新课程状态请求体（仅用于 swag 文档）
type UpdateCourseStatusRequest struct {
	Status string `json:"status" example:"published"`
}
