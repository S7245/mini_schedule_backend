package persistence

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/staff"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type staffRepository struct {
	db    *gorm.DB
	guard *commercial.SubscriptionGuard
}

// NewStaffRepository 创建 Staff 仓储；guard 在 Create 内事务里做 seats quota。
func NewStaffRepository(db *gorm.DB, guard *commercial.SubscriptionGuard) staff.Repository {
	return &staffRepository{db: db, guard: guard}
}

// brandUserWithOwnerModel 扩展 BrandUserModel 加 is_owner 列（migration 000005 已加）。
type brandUserWithOwnerModel struct {
	BrandUserModel
	IsOwner bool `gorm:"column:is_owner"`
}

func (brandUserWithOwnerModel) TableName() string { return "brand_users" }

// Create 用于 staff service：插入 brand_user + 标 is_owner=false + 写 audit。
//
// 角色 / location 分配在外层 staff service 拼装事务时调 ReplaceXxx，
// 避免本方法 30+ 参数列表。
func (r *staffRepository) Create(ctx context.Context, in staff.CreateInput) (*staff.Staff, error) {
	var created brandUserWithOwnerModel

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Subscription quota：lock active sub + COUNT brand_users + compare max_staff_seats
		if _, _, err := r.guard.CheckAndCount(ctx, tx, in.BrandID, commercial.ResourceStaff); err != nil {
			return err
		}

		created = brandUserWithOwnerModel{
			BrandUserModel: BrandUserModel{
				BrandID:      in.BrandID,
				Phone:        strings.TrimSpace(in.Phone),
				PasswordHash: in.InitialPassword, // 调用方已 bcrypt
				Name:         strings.TrimSpace(in.Name),
				Status:       string(staff.StatusActive),
			},
			IsOwner: false,
		}
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrStaffPhoneDuplicated, "手机号已被占用", 409)
			}
			return apperr.ErrInternalF("创建员工失败", err)
		}
		bID := in.BrandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: in.ActorID},
			Action:  "staff_created",
			Target:  audit.Target{Type: "staff", ID: created.ID},
			After:   &created,
		})
	})
	if err != nil {
		return nil, err
	}
	return r.GetWithAssignments(ctx, in.BrandID, created.ID)
}

func (r *staffRepository) GetByID(ctx context.Context, brandID, id int64) (*staff.Staff, error) {
	var m brandUserWithOwnerModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ? AND deleted_at IS NULL", id, brandID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询员工失败", err)
	}
	return toStaffDomain(&m, nil, nil, false, nil), nil
}

func (r *staffRepository) GetWithAssignments(ctx context.Context, brandID, id int64) (*staff.Staff, error) {
	var m brandUserWithOwnerModel
	if err := r.db.WithContext(ctx).
		Where("id = ? AND brand_id = ? AND deleted_at IS NULL", id, brandID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
		}
		return nil, apperr.ErrInternalF("查询员工失败", err)
	}

	roleAssignments, err := r.fetchRoleAssignments(ctx, brandID, []int64{id})
	if err != nil {
		return nil, err
	}
	locationAssignments, err := r.fetchLocationAssignments(ctx, brandID, []int64{id})
	if err != nil {
		return nil, err
	}
	hasInstructor, err := r.checkHasInstructor(ctx, brandID, []int64{id})
	if err != nil {
		return nil, err
	}
	// 详情内嵌教练档案（前端「教练档案」卡读 instructor_profile；缺这块导致已建档案显示「未启用」）。
	profile, err := r.fetchInstructorProfileView(ctx, brandID, id)
	if err != nil {
		return nil, err
	}

	return toStaffDomain(&m, roleAssignments[id], locationAssignments[id], hasInstructor[id], profile), nil
}

func (r *staffRepository) List(ctx context.Context, filter staff.ListFilter, offset, limit int) ([]*staff.Staff, int64, error) {
	q := r.db.WithContext(ctx).
		Model(&brandUserWithOwnerModel{}).
		Where("brand_id = ? AND deleted_at IS NULL", filter.BrandID)
	if filter.Status == string(staff.StatusActive) || filter.Status == string(staff.StatusInactive) {
		q = q.Where("status = ?", filter.Status)
	}
	if filter.Search != "" {
		s := "%" + strings.TrimSpace(filter.Search) + "%"
		q = q.Where("name LIKE ? OR phone LIKE ?", s, s)
	}

	if filter.HasInstructor != nil {
		// soft-delete JOIN 过滤：instructor_profiles 没有软删；brand_users 用主查询条件已过滤。
		if *filter.HasInstructor {
			q = q.Where("EXISTS (SELECT 1 FROM instructor_profiles ip WHERE ip.brand_user_id = brand_users.id AND ip.brand_id = ? AND ip.status = ?)", filter.BrandID, "active")
		} else {
			q = q.Where("NOT EXISTS (SELECT 1 FROM instructor_profiles ip WHERE ip.brand_user_id = brand_users.id AND ip.brand_id = ?)", filter.BrandID)
		}
	}

	// Batch 6 T07：data_scope=assigned_locations 收紧 — 只返任职在 scope location 集内的 staff。
	// nil = all_brand 不限制；空切片 = DataScopeNone 拒绝所有。
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("EXISTS (SELECT 1 FROM staff_location_assignments sla WHERE sla.brand_user_id = brand_users.id AND sla.brand_id = ? AND sla.status = 'active' AND sla.location_id IN ?)",
				filter.BrandID, filter.ScopeLocationIDs)
		}
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询员工列表失败", err)
	}

	var rows []brandUserWithOwnerModel
	if err := q.Order("id ASC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询员工列表失败", err)
	}
	if len(rows) == 0 {
		return nil, total, nil
	}

	ids := make([]int64, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	roleAssignments, err := r.fetchRoleAssignments(ctx, filter.BrandID, ids)
	if err != nil {
		return nil, 0, err
	}
	locationAssignments, err := r.fetchLocationAssignments(ctx, filter.BrandID, ids)
	if err != nil {
		return nil, 0, err
	}
	hasInstructor, err := r.checkHasInstructor(ctx, filter.BrandID, ids)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*staff.Staff, 0, len(rows))
	for i := range rows {
		items = append(items, toStaffDomain(&rows[i], roleAssignments[rows[i].ID], locationAssignments[rows[i].ID], hasInstructor[rows[i].ID], nil))
	}
	return items, total, nil
}

// Update 编辑 staff 基础字段；事务内拉 before → UPDATE → 写 audit（review B2）。
// 无变更时为 no-op，不写 audit。
func (r *staffRepository) Update(ctx context.Context, brandID, actorID, id int64, in staff.UpdateInput) (*staff.Staff, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before brandUserWithOwnerModel
		if err := tx.Where("id = ? AND brand_id = ? AND deleted_at IS NULL", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("查询员工失败", err)
		}
		updates := buildStaffUpdates(in)
		if len(updates) == 0 {
			return nil // no-op：无字段改 → 不写 audit
		}
		if err := tx.Model(&brandUserWithOwnerModel{}).
			Where("id = ?", id).
			Updates(updates).Error; err != nil {
			return apperr.ErrInternalF("更新员工失败", err)
		}
		after := before
		if in.Name != nil {
			after.Name = *in.Name
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "staff_updated",
			Target:  audit.Target{Type: "staff", ID: id},
			Before:  &before,
			After:   &after,
		})
	})
	if err != nil {
		return nil, err
	}
	return r.GetWithAssignments(ctx, brandID, id)
}

func buildStaffUpdates(in staff.UpdateInput) map[string]interface{} {
	updates := map[string]interface{}{}
	if in.Name != nil {
		updates["name"] = strings.TrimSpace(*in.Name)
	}
	return updates
}

func (r *staffRepository) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status staff.Status) (*staff.Staff, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before brandUserWithOwnerModel
		if err := tx.Where("id = ? AND brand_id = ? AND deleted_at IS NULL", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("查询员工失败", err)
		}
		if before.Status == string(status) {
			return nil
		}
		if err := tx.Model(&brandUserWithOwnerModel{}).
			Where("id = ?", id).
			Update("status", string(status)).Error; err != nil {
			return apperr.ErrInternalF("更新员工状态失败", err)
		}
		after := before
		after.Status = string(status)
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "staff_status_changed",
			Target:  audit.Target{Type: "staff", ID: id},
			Before:  &before,
			After:   &after,
		})
	})
	if err != nil {
		return nil, err
	}
	return r.GetWithAssignments(ctx, brandID, id)
}

func (r *staffRepository) SoftDelete(ctx context.Context, brandID, actorID, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before brandUserWithOwnerModel
		if err := tx.Where("id = ? AND brand_id = ? AND deleted_at IS NULL", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("查询员工失败", err)
		}
		res := tx.Where("id = ? AND brand_id = ?", id, brandID).Delete(&brandUserWithOwnerModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除员工失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "staff_deleted",
			Target:  audit.Target{Type: "staff", ID: id},
			Before:  &before,
		})
	})
}

// InScopeLocations 判断 staff 是否任职在给定 location 集内（Batch 6 T07 详情守卫）。
// locationIDs 空切片 → false（DataScopeNone）。
func (r *staffRepository) InScopeLocations(ctx context.Context, brandID, staffID int64, locationIDs []int64) (bool, error) {
	if len(locationIDs) == 0 {
		return false, nil
	}
	var count int64
	if err := r.db.WithContext(ctx).
		Table("staff_location_assignments").
		Where("brand_id = ? AND brand_user_id = ? AND status = 'active' AND location_id IN ?", brandID, staffID, locationIDs).
		Count(&count).Error; err != nil {
		return false, apperr.ErrInternalF("查询员工任职范围失败", err)
	}
	return count > 0, nil
}

func (r *staffRepository) CountActiveOwners(ctx context.Context, brandID int64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&brandUserWithOwnerModel{}).
		Where("brand_id = ? AND is_owner = TRUE AND status = ? AND deleted_at IS NULL", brandID, "active").
		Count(&count).Error; err != nil {
		return 0, apperr.ErrInternalF("统计 Owner 数量失败", err)
	}
	return count, nil
}

func (r *staffRepository) ReplaceRoleAssignments(
	ctx context.Context, brandID, actorID, brandUserID int64, items []staff.RoleAssignmentResolved,
) ([]staff.RoleAssignment, error) {
	var savedIDs []int64

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// review B5：先锁 brand_user 行，串行化同一员工的并发 PUT，避免 last-writer-wins
		// 和 audit before 漂移。同时校验 staff 存在 + 属于本 brand。
		var staffRow brandUserWithOwnerModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ? AND deleted_at IS NULL", brandUserID, brandID).
			First(&staffRow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("锁定员工失败", err)
		}

		// 单事务：拉 before（用于 audit）→ 全量 DELETE → INSERT 新 → 写 audit
		var before []BrandUserRoleAssignmentModel
		if err := tx.Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).Find(&before).Error; err != nil {
			return apperr.ErrInternalF("查询角色任职失败", err)
		}
		if err := tx.Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).Delete(&BrandUserRoleAssignmentModel{}).Error; err != nil {
			return apperr.ErrInternalF("清空角色任职失败", err)
		}
		for _, it := range items {
			row := BrandUserRoleAssignmentModel{
				BrandID:     brandID,
				BrandUserID: brandUserID,
				RoleID:      it.RoleID,
				LocationID:  it.LocationID,
				DataScope:   it.DataScope,
				Status:      "active",
			}
			if err := tx.Create(&row).Error; err != nil {
				return apperr.ErrInternalF("插入角色任职失败", err)
			}
			savedIDs = append(savedIDs, row.ID)
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "staff_role_assignments_changed",
			Target:  audit.Target{Type: "staff", ID: brandUserID},
			Before:  before,
			After:   items,
		})
	})
	if err != nil {
		return nil, err
	}

	bucket, err := r.fetchRoleAssignments(ctx, brandID, []int64{brandUserID})
	if err != nil {
		return nil, err
	}
	return bucket[brandUserID], nil
}

func (r *staffRepository) ReplaceLocationAssignments(
	ctx context.Context, brandID, actorID, brandUserID int64, items []staff.LocationAssignmentInput,
) ([]staff.LocationAssignment, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// review B5：同 ReplaceRoleAssignments，锁 brand_user 行以串行化并发 PUT。
		var staffRow brandUserWithOwnerModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND brand_id = ? AND deleted_at IS NULL", brandUserID, brandID).
			First(&staffRow).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrStaffNotFound, "员工不存在", 404)
			}
			return apperr.ErrInternalF("锁定员工失败", err)
		}

		var before []StaffLocationAssignmentModel
		if err := tx.Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).Find(&before).Error; err != nil {
			return apperr.ErrInternalF("查询 Location 任职失败", err)
		}
		if err := tx.Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).Delete(&StaffLocationAssignmentModel{}).Error; err != nil {
			return apperr.ErrInternalF("清空 Location 任职失败", err)
		}
		for _, it := range items {
			// 校验 location_id 属于本 brand 且未软删
			var loc LocationModel
			if err := tx.Where("id = ? AND brand_id = ? AND deleted_at IS NULL", it.LocationID, brandID).First(&loc).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return apperr.NewAppError(apperr.ErrLocationAssignmentInvalid, "门店任职不合法", 400)
				}
				return apperr.ErrInternalF("校验门店失败", err)
			}
			row := StaffLocationAssignmentModel{
				BrandID:        brandID,
				BrandUserID:    brandUserID,
				LocationID:     it.LocationID,
				AssignmentType: it.AssignmentType,
				IsPrimary:      it.IsPrimary,
				Status:         "active",
			}
			if err := tx.Create(&row).Error; err != nil {
				if isUniqueViolation(err) {
					return apperr.NewAppError(apperr.ErrLocationAssignmentInvalid, "门店任职重复", 400)
				}
				return apperr.ErrInternalF("插入门店任职失败", err)
			}
		}
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
			Action:  "staff_location_assignments_changed",
			Target:  audit.Target{Type: "staff", ID: brandUserID},
			Before:  before,
			After:   items,
		})
	})
	if err != nil {
		return nil, err
	}

	bucket, err := r.fetchLocationAssignments(ctx, brandID, []int64{brandUserID})
	if err != nil {
		return nil, err
	}
	return bucket[brandUserID], nil
}

// fetchRoleAssignments 一次拉一批 brand_user 的角色任职，按 brand_user_id 聚合。
// 注意：JOIN brand_roles 拿 code/name/scope_type；JOIN brand_users 加 deleted_at IS NULL 过滤。
func (r *staffRepository) fetchRoleAssignments(ctx context.Context, brandID int64, ids []int64) (map[int64][]staff.RoleAssignment, error) {
	if len(ids) == 0 {
		return map[int64][]staff.RoleAssignment{}, nil
	}
	type row struct {
		ID          int64
		BrandUserID int64
		RoleID      int64
		Code        string
		Name        string
		ScopeType   string
		LocationID  *int64
		DataScope   string
		Status      string
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("brand_user_role_assignments AS a").
		Select("a.id, a.brand_user_id, a.role_id, br.code, br.name, br.scope_type, a.location_id, a.data_scope, a.status").
		Joins("JOIN brand_roles br ON br.id = a.role_id").
		Joins("JOIN brand_users bu ON bu.id = a.brand_user_id").
		Where("a.brand_id = ? AND a.brand_user_id IN ? AND bu.deleted_at IS NULL", brandID, ids).
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询角色任职失败", err)
	}
	out := make(map[int64][]staff.RoleAssignment, len(ids))
	for _, r := range rows {
		out[r.BrandUserID] = append(out[r.BrandUserID], staff.RoleAssignment{
			ID:         r.ID,
			RoleID:     r.RoleID,
			RoleCode:   r.Code,
			RoleName:   r.Name,
			ScopeType:  r.ScopeType,
			LocationID: r.LocationID,
			DataScope:  r.DataScope,
			Status:     r.Status,
		})
	}
	return out, nil
}

func (r *staffRepository) fetchLocationAssignments(ctx context.Context, brandID int64, ids []int64) (map[int64][]staff.LocationAssignment, error) {
	if len(ids) == 0 {
		return map[int64][]staff.LocationAssignment{}, nil
	}
	type row struct {
		ID             int64
		BrandUserID    int64
		LocationID     int64
		LocationName   string
		AssignmentType string
		IsPrimary      bool
		Status         string
	}
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("staff_location_assignments AS a").
		Select("a.id, a.brand_user_id, a.location_id, l.name AS location_name, a.assignment_type, a.is_primary, a.status").
		Joins("JOIN locations l ON l.id = a.location_id").
		Joins("JOIN brand_users bu ON bu.id = a.brand_user_id").
		Where("a.brand_id = ? AND a.brand_user_id IN ? AND bu.deleted_at IS NULL AND l.deleted_at IS NULL", brandID, ids).
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询门店任职失败", err)
	}
	out := make(map[int64][]staff.LocationAssignment, len(ids))
	for _, r := range rows {
		out[r.BrandUserID] = append(out[r.BrandUserID], staff.LocationAssignment{
			ID:             r.ID,
			LocationID:     r.LocationID,
			LocationName:   r.LocationName,
			AssignmentType: r.AssignmentType,
			IsPrimary:      r.IsPrimary,
			Status:         r.Status,
		})
	}
	return out, nil
}

func (r *staffRepository) checkHasInstructor(ctx context.Context, brandID int64, ids []int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	type row struct{ BrandUserID int64 }
	var rows []row
	if err := r.db.WithContext(ctx).
		Table("instructor_profiles").
		Select("brand_user_id").
		Where("brand_id = ? AND brand_user_id IN ?", brandID, ids).
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询教练标识失败", err)
	}
	for _, r := range rows {
		out[r.BrandUserID] = true
	}
	return out, nil
}

func toStaffDomain(m *brandUserWithOwnerModel, roles []staff.RoleAssignment, locs []staff.LocationAssignment, hasInstructor bool, profile *staff.InstructorProfileView) *staff.Staff {
	// nil slice → empty slice，确保 JSON 序列化为 `[]` 而不是被丢字段（前端 .map() 防御）。
	if roles == nil {
		roles = []staff.RoleAssignment{}
	}
	if locs == nil {
		locs = []staff.LocationAssignment{}
	}
	return &staff.Staff{
		ID:                  m.ID,
		BrandID:             m.BrandID,
		Phone:               m.Phone,
		Name:                m.Name,
		Status:              staff.Status(m.Status),
		IsOwner:             m.IsOwner,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
		RoleAssignments:     roles,
		LocationAssignments: locs,
		HasInstructor:       hasInstructor,
		InstructorProfile:   profile,
	}
}

// fetchInstructorProfileView 取 brand_user 的教练档案内嵌视图（无则返 nil）。
// 详情接口用：与 GET /staff/:id/instructor 同形状（specialties/certificates split 成数组）。
func (r *staffRepository) fetchInstructorProfileView(ctx context.Context, brandID, brandUserID int64) (*staff.InstructorProfileView, error) {
	var m InstructorProfileModel
	err := r.db.WithContext(ctx).
		Where("brand_id = ? AND brand_user_id = ?", brandID, brandUserID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, apperr.ErrInternalF("查询教练档案失败", err)
	}
	return &staff.InstructorProfileView{
		ID:                  m.ID,
		BrandID:             m.BrandID,
		BrandUserID:         m.BrandUserID,
		DisplayName:         m.DisplayName,
		AvatarURL:           m.AvatarURL,
		Bio:                 m.Bio,
		Specialties:         csvToSlice(m.Specialties),
		Certificates:        csvToSlice(m.Certificates),
		IsVisibleToLearners: m.IsVisibleToLearners,
		IsSchedulable:       m.IsSchedulable,
		Status:              m.Status,
		CreatedAt:           m.CreatedAt,
		UpdatedAt:           m.UpdatedAt,
	}, nil
}

// csvToSlice DB 的 CSV（逗号/中文逗号/分号分隔）→ []string，空串返 []（非 nil，前端 .map 安全）。
func csvToSlice(s string) []string {
	out := []string{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；'
	}) {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
