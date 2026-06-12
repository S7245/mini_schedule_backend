package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// isUniqueViolationPG 用 pgconn.PgError code 23505 判定唯一约束冲突，
// 比基于错误字符串前缀的 isUniqueViolation 更可靠（后者对部分 message 格式漏判）。
func isUniqueViolationPG(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

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

// ListPermissions 返回全量 active permissions（系统级，不分 brand），按 domain、code 排序。
func (r *roleRepository) ListPermissions(ctx context.Context) ([]role.Permission, error) {
	var rows []PermissionModel
	if err := r.db.WithContext(ctx).
		Where("status = ?", "active").
		Order("domain ASC, code ASC").
		Find(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询权限列表失败", err)
	}
	out := make([]role.Permission, 0, len(rows))
	for i := range rows {
		out = append(out, role.Permission{
			ID:     rows[i].ID,
			Code:   rows[i].Code,
			Domain: rows[i].Domain,
			Action: rows[i].Action,
			Name:   rows[i].Name,
			Status: rows[i].Status,
		})
	}
	return out, nil
}

// resolvePermissionIDs 把原始权限 code 集合解析成 permission_id 列表（去重、保序无关）。
// 未知 code 视为非法参数（前端应只从 GET /permissions 取值）。
func resolvePermissionIDs(ctx context.Context, tx *gorm.DB, codes []string) ([]int64, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	// 去重
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(codes))
	for _, c := range codes {
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		uniq = append(uniq, c)
	}
	var rows []PermissionModel
	if err := tx.WithContext(ctx).
		Where("code IN ? AND status = ?", uniq, "active").
		Find(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("解析权限 code 失败", err)
	}
	if len(rows) != len(uniq) {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "存在未知或已停用的权限 code", 400)
	}
	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	return ids, nil
}

// generateCustomCode 生成 custom_<8 hex> 形式的角色 code。
func generateCustomCode() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "custom_" + hex.EncodeToString(buf), nil
}

// CreateBrandRole 新建自定义角色（is_system=FALSE），事务内插 brand_roles +
// brand_role_permissions（存原始 code 解析出的 permission_id，不展开）+ 写 audit。
func (r *roleRepository) CreateBrandRole(ctx context.Context, in role.CreateBrandRoleInput) (*role.BrandRole, error) {
	var newRoleID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		permIDs, err := resolvePermissionIDs(ctx, tx, in.PermissionCodes)
		if err != nil {
			return err
		}

		code, err := generateCustomCode()
		if err != nil {
			return apperr.ErrInternalF("生成角色 code 失败", err)
		}

		// 用 raw INSERT（与 role_allocator 同模式）显式写 is_system=FALSE，
		// 避免 GORM 把 false 当零值省略落到 DB default:true。
		if err := tx.WithContext(ctx).Raw(
			`INSERT INTO brand_roles (brand_id, code, name, scope_type, is_system, status, description)
			 VALUES (?, ?, ?, ?, FALSE, 'active', ?) RETURNING id`,
			in.BrandID, code, in.Name, in.ScopeType, in.Description,
		).Scan(&newRoleID).Error; err != nil {
			if isUniqueViolationPG(err) {
				return apperr.NewAppError(apperr.ErrRoleCodeDuplicated, "角色 code 冲突，请重试", 409)
			}
			return apperr.ErrInternalF("创建角色失败", err)
		}

		if err := insertBrandRolePermissions(tx, in.BrandID, newRoleID, permIDs); err != nil {
			return err
		}

		bID := in.BrandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: in.ActorID},
			Action:  "brand_role_created",
			Target:  audit.Target{Type: "brand_role", ID: newRoleID},
			After:   map[string]any{"code": code, "name": in.Name, "scope_type": in.ScopeType, "permission_codes": in.PermissionCodes},
		})
	})
	if err != nil {
		return nil, err
	}
	return r.getBrandRoleByIDWithPermissions(ctx, in.BrandID, newRoleID)
}

// UpdateBrandRole 全量替换 name/description + permissions（scope_type 不变）。
func (r *roleRepository) UpdateBrandRole(ctx context.Context, in role.UpdateBrandRoleInput) (*role.BrandRole, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandRoleModel
		if err := tx.Where("id = ? AND brand_id = ?", in.RoleID, in.BrandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
			}
			return apperr.ErrInternalF("查询角色失败", err)
		}

		permIDs, err := resolvePermissionIDs(ctx, tx, in.PermissionCodes)
		if err != nil {
			return err
		}

		if err := tx.Model(&BrandRoleModel{}).
			Where("id = ?", in.RoleID).
			Updates(map[string]interface{}{
				"name":        in.Name,
				"description": in.Description,
			}).Error; err != nil {
			return apperr.ErrInternalF("更新角色失败", err)
		}

		// 全量替换权限：DELETE → INSERT。
		if err := tx.Where("role_id = ?", in.RoleID).Delete(&BrandRolePermissionModel{}).Error; err != nil {
			return apperr.ErrInternalF("清空角色权限失败", err)
		}
		if err := insertBrandRolePermissions(tx, in.BrandID, in.RoleID, permIDs); err != nil {
			return err
		}

		bID := in.BrandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: in.ActorID},
			Action:  "brand_role_updated",
			Target:  audit.Target{Type: "brand_role", ID: in.RoleID},
			Before:  map[string]any{"name": before.Name, "description": before.Description},
			After:   map[string]any{"name": in.Name, "description": in.Description, "permission_codes": in.PermissionCodes},
		})
	})
	if err != nil {
		return nil, err
	}
	return r.getBrandRoleByIDWithPermissions(ctx, in.BrandID, in.RoleID)
}

// UpdateBrandRoleStatus 切换 active / inactive。
func (r *roleRepository) UpdateBrandRoleStatus(ctx context.Context, brandID, actorID, roleID int64, status string) (*role.BrandRole, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandRoleModel
		if err := tx.Where("id = ? AND brand_id = ?", roleID, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
			}
			return apperr.ErrInternalF("查询角色失败", err)
		}
		if before.Status == status {
			return nil
		}
		if err := tx.Model(&BrandRoleModel{}).
			Where("id = ?", roleID).
			Update("status", status).Error; err != nil {
			return apperr.ErrInternalF("更新角色状态失败", err)
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "brand_role_status_changed",
			Target:  audit.Target{Type: "brand_role", ID: roleID},
			Before:  map[string]any{"status": before.Status},
			After:   map[string]any{"status": status},
		})
	})
	if err != nil {
		return nil, err
	}
	return r.getBrandRoleByIDWithPermissions(ctx, brandID, roleID)
}

// DeleteBrandRole 硬删角色（brand_role_permissions 走 ON DELETE CASCADE）。
func (r *roleRepository) DeleteBrandRole(ctx context.Context, brandID, actorID, roleID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandRoleModel
		if err := tx.Where("id = ? AND brand_id = ?", roleID, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
			}
			return apperr.ErrInternalF("查询角色失败", err)
		}
		res := tx.Where("id = ? AND brand_id = ?", roleID, brandID).Delete(&BrandRoleModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除角色失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "brand_role_deleted",
			Target:  audit.Target{Type: "brand_role", ID: roleID},
			Before:  map[string]any{"code": before.Code, "name": before.Name},
		})
	})
}

// CountActiveAssignmentsByRole 统计仍引用该 role 的 active 任职数（A4 删除前置校验）。
func (r *roleRepository) CountActiveAssignmentsByRole(ctx context.Context, roleID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&BrandUserRoleAssignmentModel{}).
		Where("role_id = ? AND status = ?", roleID, "active").
		Count(&count).Error; err != nil {
		return 0, apperr.ErrInternalF("统计角色任职引用失败", err)
	}
	return count, nil
}

// ListBrandUserIDsByRole 反查持有该 role 的全部 active 任职的 brand_user_id（去重），
// 供缓存批量失效（C1）。
func (r *roleRepository) ListBrandUserIDsByRole(ctx context.Context, roleID int64) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).
		Model(&BrandUserRoleAssignmentModel{}).
		Distinct("brand_user_id").
		Where("role_id = ? AND status = ?", roleID, "active").
		Pluck("brand_user_id", &ids).Error; err != nil {
		return nil, apperr.ErrInternalF("反查角色持有人失败", err)
	}
	return ids, nil
}

// insertBrandRolePermissions 批量插 brand_role_permissions（permIDs 已去重 / 已解析）。
func insertBrandRolePermissions(tx *gorm.DB, brandID, roleID int64, permIDs []int64) error {
	if len(permIDs) == 0 {
		return nil
	}
	rows := make([]BrandRolePermissionModel, 0, len(permIDs))
	for _, pid := range permIDs {
		rows = append(rows, BrandRolePermissionModel{
			BrandID:      brandID,
			RoleID:       roleID,
			PermissionID: pid,
		})
	}
	if err := tx.Create(&rows).Error; err != nil {
		return apperr.ErrInternalF("插入角色权限失败", err)
	}
	return nil
}

// getBrandRoleByIDWithPermissions 拉单角色 + 其权限明细（创建 / 更新后回查）。
func (r *roleRepository) getBrandRoleByIDWithPermissions(ctx context.Context, brandID, roleID int64) (*role.BrandRole, error) {
	var m BrandRoleModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ?", roleID, brandID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrRoleNotFound, "角色不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询角色失败", err)
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
		Where("brp.role_id = ?", roleID).
		Scan(&perms).Error; err != nil {
		return nil, apperr.ErrInternalF("查询角色权限失败", err)
	}
	out := make([]role.Permission, 0, len(perms))
	for _, p := range perms {
		out = append(out, role.Permission{
			ID:     p.PermissionModel.ID,
			Code:   p.PermissionModel.Code,
			Domain: p.PermissionModel.Domain,
			Action: p.PermissionModel.Action,
			Name:   p.PermissionModel.Name,
			Status: p.PermissionModel.Status,
		})
	}
	return toBrandRoleDomain(&m, out), nil
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
