package persistence

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type roleRepository struct {
	db *gorm.DB
}

// NewRoleRepository 创建角色 / 权限只读仓储。
func NewRoleRepository(db *gorm.DB) role.Repository {
	return &roleRepository{db: db}
}

func (r *roleRepository) ListBrandRoles(ctx context.Context, brandID int64) ([]*role.BrandRole, error) {
	var rows []BrandRoleModel
	if err := r.db.WithContext(ctx).
		Where("brand_id = ?", brandID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询品牌角色失败", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// 一次性把权限也拉回来，避免 N+1。
	roleIDs := make([]int64, 0, len(rows))
	for i := range rows {
		roleIDs = append(roleIDs, rows[i].ID)
	}

	type permRow struct {
		RoleID int64
		PermissionModel
	}
	var perms []permRow
	if err := r.db.WithContext(ctx).
		Table("brand_role_permissions AS brp").
		Select("brp.role_id, p.*").
		Joins("JOIN permissions p ON p.id = brp.permission_id").
		Where("brp.role_id IN ?", roleIDs).
		Scan(&perms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询角色权限失败", err)
	}

	bucket := map[int64][]role.Permission{}
	for _, p := range perms {
		bucket[p.RoleID] = append(bucket[p.RoleID], role.Permission{
			ID:     p.PermissionModel.ID,
			Code:   p.PermissionModel.Code,
			Domain: p.PermissionModel.Domain,
			Action: p.PermissionModel.Action,
			Name:   p.PermissionModel.Name,
			Status: p.PermissionModel.Status,
		})
	}

	out := make([]*role.BrandRole, 0, len(rows))
	for i := range rows {
		out = append(out, toBrandRoleDomain(&rows[i], bucket[rows[i].ID]))
	}
	return out, nil
}

func (r *roleRepository) GetBrandRoleByCode(ctx context.Context, brandID int64, code string) (*role.BrandRole, error) {
	var m BrandRoleModel
	if err := r.db.WithContext(ctx).
		Where("brand_id = ? AND code = ?", brandID, code).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询角色失败", err)
	}
	return toBrandRoleDomain(&m, nil), nil
}

func (r *roleRepository) ListRoleTemplatesWithPermissions(ctx context.Context) ([]*role.RoleTemplate, error) {
	var templates []RoleTemplateModel
	if err := r.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("id ASC").
		Find(&templates).Error; err != nil {
		return nil, apperr.ErrInternalF("查询角色模板失败", err)
	}
	if len(templates) == 0 {
		return nil, nil
	}

	templateIDs := make([]int64, 0, len(templates))
	for i := range templates {
		templateIDs = append(templateIDs, templates[i].ID)
	}

	type permRow struct {
		TemplateID int64
		PermissionModel
	}
	var perms []permRow
	if err := r.db.WithContext(ctx).
		Table("role_template_permissions AS rtp").
		Select("rtp.template_id, p.*").
		Joins("JOIN permissions p ON p.id = rtp.permission_id").
		Where("rtp.template_id IN ?", templateIDs).
		Scan(&perms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询模板权限失败", err)
	}

	bucket := map[int64][]role.Permission{}
	for _, p := range perms {
		bucket[p.TemplateID] = append(bucket[p.TemplateID], role.Permission{
			ID:     p.PermissionModel.ID,
			Code:   p.PermissionModel.Code,
			Domain: p.PermissionModel.Domain,
			Action: p.PermissionModel.Action,
			Name:   p.PermissionModel.Name,
			Status: p.PermissionModel.Status,
		})
	}

	out := make([]*role.RoleTemplate, 0, len(templates))
	for i := range templates {
		t := &templates[i]
		out = append(out, &role.RoleTemplate{
			ID:          t.ID,
			Code:        t.Code,
			Name:        t.Name,
			ScopeType:   t.ScopeType,
			Description: t.Description,
			Status:      t.Status,
			Permissions: bucket[t.ID],
		})
	}
	return out, nil
}

func toBrandRoleDomain(m *BrandRoleModel, perms []role.Permission) *role.BrandRole {
	return &role.BrandRole{
		ID:          m.ID,
		BrandID:     m.BrandID,
		TemplateID:  m.TemplateID,
		Code:        m.Code,
		Name:        m.Name,
		ScopeType:   m.ScopeType,
		IsSystem:    m.IsSystem,
		Status:      m.Status,
		Description: m.Description,
		Permissions: perms,
	}
}
