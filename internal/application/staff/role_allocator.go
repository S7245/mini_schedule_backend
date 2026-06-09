package staff

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/role"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// RoleAllocator 负责：
//  1) 注册流程：把 8 个 role_templates 复制到 brand_roles + 把 owner brand_user 关联到 "brand_owner"
//  2) admin backfill：遍历 brand_users.is_owner=true 调 EnsureBrandRolesSeeded + AssignDefaultOwnerRoles
type RoleAllocator struct {
	roleRepo role.Repository
	db       *gorm.DB
}

// NewRoleAllocator 创建分配器。
func NewRoleAllocator(roleRepo role.Repository, db *gorm.DB) *RoleAllocator {
	return &RoleAllocator{roleRepo: roleRepo, db: db}
}

// EnsureBrandRolesSeeded 幂等：若 brand 还没复制过预置角色，从 role_templates 一次性复制 8 个 brand_roles
// + 复制 role_template_permissions → brand_role_permissions。已经存在则 no-op。
//
// 必须在外部事务里调（避免半完成状态）。
func (a *RoleAllocator) EnsureBrandRolesSeeded(ctx context.Context, tx *gorm.DB, brandID int64) error {
	// 检查是否已经有 brand_owner 角色
	type r struct{ Cnt int64 }
	var existing r
	if err := tx.WithContext(ctx).
		Raw("SELECT COUNT(*) AS cnt FROM brand_roles WHERE brand_id = ? AND code = 'brand_owner'", brandID).
		Scan(&existing).Error; err != nil {
		return apperr.ErrInternalF("查询品牌角色失败", err)
	}
	if existing.Cnt > 0 {
		return nil
	}

	templates, err := a.roleRepo.ListRoleTemplatesWithPermissions(ctx)
	if err != nil {
		return err
	}
	if len(templates) == 0 {
		return apperr.ErrInternalF("缺少 role_templates 数据，无法初始化品牌角色", nil)
	}

	for _, t := range templates {
		// brand_roles row
		var newRoleID int64
		if err := tx.WithContext(ctx).
			Raw(`INSERT INTO brand_roles (brand_id, template_id, code, name, scope_type, is_system, status, description)
				 VALUES (?, ?, ?, ?, ?, TRUE, 'active', ?) RETURNING id`,
				brandID, t.ID, t.Code, t.Name, t.ScopeType, t.Description).
			Scan(&newRoleID).Error; err != nil {
			return apperr.ErrInternalF("复制 brand_role 失败", err)
		}
		// brand_role_permissions rows
		for _, p := range t.Permissions {
			if err := tx.WithContext(ctx).
				Exec(`INSERT INTO brand_role_permissions (brand_id, role_id, permission_id)
					 VALUES (?, ?, ?)
					 ON CONFLICT DO NOTHING`,
					brandID, newRoleID, p.ID).Error; err != nil {
				return apperr.ErrInternalF("复制 brand_role_permission 失败", err)
			}
		}
	}
	return nil
}

// AssignDefaultOwnerRolesTx 适配 commercial.OwnerRoleAllocator 接口，让 commercial.Service
// 在 brand_user INSERT 同事务内调用本逻辑。tx 必须是 *gorm.DB。
func (a *RoleAllocator) AssignDefaultOwnerRolesTx(tx any, brandID, brandUserID int64) error {
	gtx, ok := tx.(*gorm.DB)
	if !ok {
		return apperr.ErrInternalF("AssignDefaultOwnerRolesTx: tx 非 *gorm.DB", nil)
	}
	return a.AssignDefaultOwnerRoles(context.Background(), gtx, brandID, brandUserID)
}

// AssignDefaultOwnerRoles 给指定 brand_user 分配 "brand_owner" 角色（scope=all_brand）。
// 幂等：通过 ON CONFLICT DO NOTHING 兼容重复执行。
//
// 必须在外部事务里调。
func (a *RoleAllocator) AssignDefaultOwnerRoles(ctx context.Context, tx *gorm.DB, brandID, brandUserID int64) error {
	if err := a.EnsureBrandRolesSeeded(ctx, tx, brandID); err != nil {
		return err
	}

	// 查 brand_owner 角色 id
	type r struct{ ID int64 }
	var ownerRole r
	if err := tx.WithContext(ctx).
		Raw("SELECT id FROM brand_roles WHERE brand_id = ? AND code = 'brand_owner'", brandID).
		Scan(&ownerRole).Error; err != nil {
		return apperr.ErrInternalF("查询 brand_owner 角色失败", err)
	}
	if ownerRole.ID == 0 {
		return apperr.ErrInternalF("brand_owner 角色缺失", nil)
	}

	// INSERT 关联（幂等）
	if err := tx.WithContext(ctx).Exec(
		`INSERT INTO brand_user_role_assignments
			(brand_id, brand_user_id, role_id, location_id, data_scope, status, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, 'all_brand', 'active', NOW(), NOW())
		 ON CONFLICT DO NOTHING`,
		brandID, brandUserID, ownerRole.ID,
	).Error; err != nil {
		return apperr.ErrInternalF("分配品牌负责人角色失败", err)
	}

	// 标 is_owner（幂等）
	if err := tx.WithContext(ctx).Exec(
		"UPDATE brand_users SET is_owner = TRUE WHERE id = ? AND brand_id = ?",
		brandUserID, brandID,
	).Error; err != nil {
		return apperr.ErrInternalF("标记品牌负责人失败", err)
	}
	return nil
}

// BackfillResult admin backfill 接口的返回统计。
type BackfillResult struct {
	Processed int `json:"processed"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

// BackfillOwnerRoles 遍历所有 is_owner=true 的 brand_user 调 AssignDefaultOwnerRoles。
// 幂等：第二次跑同一 brand 应该 skip（看 brand_user_role_assignments 是否已经有 brand_owner）。
func (a *RoleAllocator) BackfillOwnerRoles(ctx context.Context, actorID int64) (*BackfillResult, error) {
	type row struct {
		ID      int64
		BrandID int64
	}
	var owners []row
	if err := a.db.WithContext(ctx).
		Raw("SELECT id, brand_id FROM brand_users WHERE is_owner = TRUE AND deleted_at IS NULL").
		Scan(&owners).Error; err != nil {
		return nil, apperr.ErrInternalF("查询 owner 列表失败", err)
	}

	res := &BackfillResult{}
	for _, o := range owners {
		err := a.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 已分配则跳过：查 brand_user_role_assignments 是否已有 brand_owner 关联
			type r struct{ Cnt int64 }
			var existing r
			if err := tx.Raw(`SELECT COUNT(*) AS cnt
				FROM brand_user_role_assignments a
				JOIN brand_roles br ON br.id = a.role_id
				WHERE a.brand_user_id = ? AND br.code = 'brand_owner' AND a.status = 'active'`,
				o.ID).Scan(&existing).Error; err != nil {
				return err
			}
			if existing.Cnt > 0 {
				return errSkipBackfill
			}
			if err := a.AssignDefaultOwnerRoles(ctx, tx, o.BrandID, o.ID); err != nil {
				return err
			}
			bID := o.BrandID
			return audit.Write(tx, audit.Event{
				BrandID: &bID,
				Actor:   audit.Actor{Type: audit.ActorPlatformAdmin, ID: actorID},
				Action:  "owner_role_backfilled",
				Target:  audit.Target{Type: "brand_user", ID: o.ID},
				After:   map[string]any{"brand_id": o.BrandID},
			})
		})
		if errors.Is(err, errSkipBackfill) {
			res.Skipped++
			continue
		}
		if err != nil {
			res.Failed++
			continue
		}
		res.Processed++
	}
	return res, nil
}

var errSkipBackfill = errors.New("backfill: already assigned")
