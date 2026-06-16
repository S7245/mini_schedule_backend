package brand

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	appStaff "github.com/zkw/mini-schedule/backend/internal/application/staff"
	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	"github.com/zkw/mini-schedule/backend/internal/domain/staff"
	"github.com/zkw/mini-schedule/backend/internal/interfaces/middleware"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/response"
)

// StaffHandler brand 端员工接口（11 endpoints）。
type StaffHandler struct {
	svc *appStaff.Service
}

func NewStaffHandler(svc *appStaff.Service) *StaffHandler {
	return &StaffHandler{svc: svc}
}

func (h *StaffHandler) RegisterRoutes(g *gin.RouterGroup) {
	g.GET("/staff", h.list)
	g.GET("/staff/:id", h.get)
	g.POST("/staff", h.create)
	g.PATCH("/staff/:id", h.update)
	g.PATCH("/staff/:id/status", h.updateStatus)
	g.DELETE("/staff/:id", h.delete)
	g.PUT("/staff/:id/role-assignments", h.replaceRoleAssignments)
	g.PUT("/staff/:id/location-assignments", h.replaceLocationAssignments)
	g.GET("/instructors", h.listInstructors)
	g.GET("/staff/:id/instructor", h.getInstructor)
	g.PUT("/staff/:id/instructor", h.upsertInstructor)
	g.DELETE("/staff/:id/instructor", h.deleteInstructor)
	g.GET("/roles", h.listRoles)
	g.GET("/roles/:id", h.getRole)
	g.POST("/roles", h.createRole)
	g.PUT("/roles/:id", h.updateRole)
	g.PATCH("/roles/:id/status", h.patchRoleStatus)
	g.DELETE("/roles/:id", h.deleteRole)
	g.GET("/permissions", h.listPermissions)
}

func (h *StaffHandler) list(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	status := c.Query("status")
	search := c.Query("search")

	var hasInstr *bool
	if v := c.Query("with_instructor"); v != "" {
		b := v == "true" || v == "1"
		hasInstr = &b
	}

	items, total, err := h.svc.List(c.Request.Context(), appStaff.ListInput{
		BrandID:       brandID,
		ActorID:       middleware.GetUserID(c),
		Status:        status,
		HasInstructor: hasInstr,
		Search:        search,
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.SuccessPage(c, items, total, page, pageSize)
}

func (h *StaffHandler) get(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	s, err := h.svc.Get(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

type createStaffBody struct {
	Phone               string                           `json:"phone"`
	Name                string                           `json:"name"`
	InitialPassword     string                           `json:"initial_password"`
	RoleCodes           []string                         `json:"role_codes"`
	LocationAssignments []staffLocationAssignmentReqBody `json:"location_assignments"`
}

type staffLocationAssignmentReqBody struct {
	LocationID     int64  `json:"location_id"`
	AssignmentType string `json:"assignment_type"`
	IsPrimary      bool   `json:"is_primary"`
}

func (h *StaffHandler) create(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	in := appStaff.CreateInput{
		BrandID:         brandID,
		ActorID:         actorID,
		Phone:           body.Phone,
		Name:            body.Name,
		InitialPassword: body.InitialPassword,
		RoleCodes:       body.RoleCodes,
	}
	for _, la := range body.LocationAssignments {
		in.LocationAssignments = append(in.LocationAssignments, staff.LocationAssignmentInput{
			LocationID:     la.LocationID,
			AssignmentType: la.AssignmentType,
			IsPrimary:      la.IsPrimary,
		})
	}
	s, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": s})
}

type updateStaffBody struct {
	Name *string `json:"name"`
}

func (h *StaffHandler) update(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateStaffBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	s, err := h.svc.Update(c.Request.Context(), brandID, actorID, id, staff.UpdateInput{Name: body.Name})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

type updateStaffStatusBody struct {
	Status string `json:"status"`
}

func (h *StaffHandler) updateStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateStaffStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	s, err := h.svc.UpdateStatus(c.Request.Context(), brandID, actorID, id, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

func (h *StaffHandler) delete(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), brandID, actorID, id); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type roleAssignmentItemBody struct {
	RoleCode   string `json:"role_code"`
	LocationID *int64 `json:"location_id,omitempty"`
	DataScope  string `json:"data_scope,omitempty"`
}
type replaceRoleAssignmentsBody struct {
	Assignments []roleAssignmentItemBody `json:"assignments"`
}

func (h *StaffHandler) replaceRoleAssignments(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body replaceRoleAssignmentsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	items := make([]staff.RoleAssignmentInput, 0, len(body.Assignments))
	for _, it := range body.Assignments {
		items = append(items, staff.RoleAssignmentInput{
			RoleCode:   it.RoleCode,
			LocationID: it.LocationID,
			DataScope:  it.DataScope,
		})
	}
	s, err := h.svc.ReplaceRoleAssignments(c.Request.Context(), brandID, actorID, id, items)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

type replaceLocationAssignmentsBody struct {
	Assignments []staffLocationAssignmentReqBody `json:"assignments"`
}

func (h *StaffHandler) replaceLocationAssignments(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body replaceLocationAssignmentsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	items := make([]staff.LocationAssignmentInput, 0, len(body.Assignments))
	for _, la := range body.Assignments {
		items = append(items, staff.LocationAssignmentInput{
			LocationID:     la.LocationID,
			AssignmentType: la.AssignmentType,
			IsPrimary:      la.IsPrimary,
		})
	}
	s, err := h.svc.ReplaceLocationAssignments(c.Request.Context(), brandID, actorID, id, items)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

// schedulableInstructorItem 排课弹窗下拉用的精简教练投影。
// id 即 instructor_profile_id，与 POST /class-sessions 的 instructor_profile_id 入参对齐。
type schedulableInstructorItem struct {
	ID            int64  `json:"id"`
	DisplayName   string `json:"display_name"`
	Status        string `json:"status"`
	IsSchedulable bool   `json:"is_schedulable"`
}

// listInstructors GET /instructors[?schedulable=true]
// 当前仅服务排课弹窗，固定返回本 brand 下 active + 可排课的教练。
func (h *StaffHandler) listInstructors(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	profiles, err := h.svc.ListSchedulableInstructors(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	items := make([]schedulableInstructorItem, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, schedulableInstructorItem{
			ID:            p.ID,
			DisplayName:   p.DisplayName,
			Status:        string(p.Status),
			IsSchedulable: p.IsSchedulable,
		})
	}
	response.Success(c, gin.H{"items": items})
}

func (h *StaffHandler) getInstructor(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	p, err := h.svc.GetInstructor(c.Request.Context(), brandID, actorID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, toInstructorResponse(p))
}

type upsertInstructorBody struct {
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Bio         string `json:"bio"`
	// 接受数组 string[]（前端 chip / 输入控件惯例），handler 内部 join 成 csv 给 service / DB。
	Specialties         []string `json:"specialties"`
	Certificates        []string `json:"certificates"`
	IsVisibleToLearners bool     `json:"is_visible_to_learners"`
	IsSchedulable       bool     `json:"is_schedulable"`
	Status              string   `json:"status"`
}

// instructorProfileResponse 把 domain 的 csv 字符串 split 回数组，与前端 string[] 类型对齐。
type instructorProfileResponse struct {
	*instructor.Profile
	Specialties  []string `json:"specialties"`
	Certificates []string `json:"certificates"`
}

func toInstructorResponse(p *instructor.Profile) *instructorProfileResponse {
	if p == nil {
		return nil
	}
	return &instructorProfileResponse{
		Profile:      p,
		Specialties:  splitCSV(p.Specialties),
		Certificates: splitCSV(p.Certificates),
	}
}

// joinCSV 把 []string 合成逗号分隔字符串（trim + 跳过空项），存 DB 用。
func joinCSV(items []string) string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

// splitCSV 反向操作，DB csv → []string（空字符串返 []）。
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (h *StaffHandler) upsertInstructor(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body upsertInstructorBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	prof, err := h.svc.UpsertInstructor(c.Request.Context(), brandID, actorID, id, instructor.UpsertInput{
		DisplayName:         body.DisplayName,
		AvatarURL:           body.AvatarURL,
		Bio:                 body.Bio,
		Specialties:         joinCSV(body.Specialties),
		Certificates:        joinCSV(body.Certificates),
		IsVisibleToLearners: body.IsVisibleToLearners,
		IsSchedulable:       body.IsSchedulable,
		Status:              instructor.Status(body.Status),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, toInstructorResponse(prof))
}

func (h *StaffHandler) deleteInstructor(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	if err := h.svc.DeleteInstructor(c.Request.Context(), brandID, actorID, id); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *StaffHandler) listRoles(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	roles, err := h.svc.ListRoles(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, roles)
}

// roleCodeParam 取 :id 段作为角色 code（角色用字符串 code 寻址，非数字 ID）。
func roleCodeParam(c *gin.Context) (string, error) {
	code := strings.TrimSpace(c.Param("id"))
	if code == "" {
		return "", apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
	}
	return code, nil
}

func (h *StaffHandler) getRole(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	code, err := roleCodeParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	r, err := h.svc.GetRole(c.Request.Context(), brandID, actorID, code)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, r)
}

// createRoleBody POST /roles 请求体。
// permission_codes 不加 omitempty：handler 收 []string，允许空数组（Batch 5 坑）。
type createRoleBody struct {
	Name            string   `json:"name"`
	ScopeType       string   `json:"scope_type"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
}

func (h *StaffHandler) createRole(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	var body createRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if body.PermissionCodes == nil {
		body.PermissionCodes = []string{}
	}
	r, err := h.svc.CreateRole(c.Request.Context(), appStaff.CreateRoleInput{
		BrandID:         brandID,
		ActorID:         actorID,
		Name:            body.Name,
		ScopeType:       body.ScopeType,
		Description:     body.Description,
		PermissionCodes: body.PermissionCodes,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"code": "OK", "message": "created", "data": r})
}

// updateRoleBody PUT /roles/:code 请求体。无 scope_type（A3 不可改）。
type updateRoleBody struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	PermissionCodes []string `json:"permission_codes"`
}

func (h *StaffHandler) updateRole(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	code, err := roleCodeParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body updateRoleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	if body.PermissionCodes == nil {
		body.PermissionCodes = []string{}
	}
	r, err := h.svc.UpdateRole(c.Request.Context(), appStaff.UpdateRoleInput{
		BrandID:         brandID,
		ActorID:         actorID,
		Code:            code,
		Name:            body.Name,
		Description:     body.Description,
		PermissionCodes: body.PermissionCodes,
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, r)
}

type patchRoleStatusBody struct {
	Status string `json:"status"`
}

func (h *StaffHandler) patchRoleStatus(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	code, err := roleCodeParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var body patchRoleStatusBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, response.ErrInvalidRequest("请求参数错误"))
		return
	}
	r, err := h.svc.PatchRoleStatus(c.Request.Context(), brandID, actorID, code, body.Status)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, r)
}

func (h *StaffHandler) deleteRole(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	code, err := roleCodeParam(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	if err := h.svc.DeleteRole(c.Request.Context(), brandID, actorID, code); err != nil {
		response.Error(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// permissionGroupResponse GET /permissions 按 domain 分组的响应单元。
type permissionGroupResponse struct {
	Domain      string                   `json:"domain"`
	Permissions []permissionItemResponse `json:"permissions"`
}

type permissionItemResponse struct {
	Code        string `json:"code"`
	Action      string `json:"action"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *StaffHandler) listPermissions(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	actorID := middleware.GetUserID(c)
	perms, err := h.svc.ListPermissions(c.Request.Context(), brandID, actorID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, groupPermissionsByDomain(perms))
}

// groupPermissionsByDomain 把扁平权限列表按 domain 分组，保持 service 已排好的
// domain/code 顺序（service ListPermissions 已 ORDER BY domain, code）。
func groupPermissionsByDomain(perms []role.Permission) []permissionGroupResponse {
	groups := make([]permissionGroupResponse, 0)
	idx := map[string]int{}
	for _, p := range perms {
		i, ok := idx[p.Domain]
		if !ok {
			groups = append(groups, permissionGroupResponse{Domain: p.Domain})
			i = len(groups) - 1
			idx[p.Domain] = i
		}
		groups[i].Permissions = append(groups[i].Permissions, permissionItemResponse{
			Code:        p.Code,
			Action:      p.Action,
			Name:        p.Name,
			Description: p.Description,
		})
	}
	return groups
}

func parseStaffID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
	}
	return id, nil
}
