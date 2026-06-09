package brand

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	appStaff "github.com/zkw/mini-schedule/backend/internal/application/staff"
	"github.com/zkw/mini-schedule/backend/internal/domain/instructor"
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
	g.GET("/staff/:id/instructor", h.getInstructor)
	g.PUT("/staff/:id/instructor", h.upsertInstructor)
	g.DELETE("/staff/:id/instructor", h.deleteInstructor)
	g.GET("/roles", h.listRoles)
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
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	s, err := h.svc.Get(c.Request.Context(), brandID, id)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, s)
}

type createStaffBody struct {
	Phone               string                              `json:"phone"`
	Name                string                              `json:"name"`
	InitialPassword     string                              `json:"initial_password"`
	RoleCodes           []string                            `json:"role_codes"`
	LocationAssignments []staffLocationAssignmentReqBody    `json:"location_assignments"`
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

func (h *StaffHandler) getInstructor(c *gin.Context) {
	brandID := middleware.GetBrandID(c)
	id, err := parseStaffID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	p, err := h.svc.GetInstructor(c.Request.Context(), brandID, id)
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
	roles, err := h.svc.ListRoles(c.Request.Context(), brandID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, roles)
}

func parseStaffID(c *gin.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
	}
	return id, nil
}
