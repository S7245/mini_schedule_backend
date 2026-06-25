package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/application/commercial"
	"github.com/zkw/mini-schedule/backend/internal/audit"
	"github.com/zkw/mini-schedule/backend/internal/domain/learner"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type learnerRepository struct {
	db    *gorm.DB
	guard *commercial.SubscriptionGuard
}

// NewLearnerRepository 创建学员仓储。guard 注入做 ResourceLearner quota（同 NewLocationRepository）。
func NewLearnerRepository(db *gorm.DB, guard *commercial.SubscriptionGuard) learner.Repository {
	return &learnerRepository{db: db, guard: guard}
}

// uniqueConstraint 返回 23505 唯一冲突的约束名。brand_learner_profiles 有两条 unique
// （brand_identity / brand_learner_no），必须按约束名分流（镜像 12a exclusionConstraint）。
func uniqueConstraint(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return pgErr.ConstraintName, true
		}
	}
	return "", false
}

// profileConflictError 按约束名把 profile 唯一冲突分流成业务错误。
func profileConflictError(err error) error {
	name, ok := uniqueConstraint(err)
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(name, "brand_learner_no"):
		return apperr.NewAppError(apperr.ErrLearnerNoDuplicated, "学号已存在", 409)
	case strings.Contains(name, "brand_identity"):
		return apperr.NewAppError(apperr.ErrLearnerAlreadyExists, "该手机号在本品牌已有学员档案", 409)
	default:
		// 约束名缺失（极少）退化为「已存在」，避免裸 500。
		return apperr.NewAppError(apperr.ErrLearnerAlreadyExists, "学员已存在", 409)
	}
}

func nullableStr(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}

func (r *learnerRepository) Create(ctx context.Context, in learner.CreateInput) (*learner.Profile, error) {
	var createdID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) quota：锁 active subscription + COUNT brand_learner_profiles + 比 max_learners。
		if _, _, err := r.guard.CheckAndCount(ctx, tx, in.BrandID, commercial.ResourceLearner); err != nil {
			return err
		}

		// 2) find-or-create identity by phone（全局唯一，跨品牌复用）。
		phone := strings.TrimSpace(in.Phone)
		identityID, err := r.findOrCreateIdentity(tx, phone, strings.TrimSpace(in.Nickname))
		if err != nil {
			return err
		}

		// 3) INSERT profile。
		profile := BrandLearnerProfileModel{
			BrandID:           in.BrandID,
			LearnerIdentityID: identityID,
			PrimaryLocationID: in.PrimaryLocationID,
			LearnerNo:         nullableStr(in.LearnerNo),
			Nickname:          strings.TrimSpace(in.Nickname),
			Remark:            strings.TrimSpace(in.Remark),
			Status:            string(learner.StatusActive),
		}
		if err := tx.Create(&profile).Error; err != nil {
			if be := profileConflictError(err); be != nil {
				return be
			}
			return apperr.ErrInternalF("创建学员失败", err)
		}
		createdID = profile.ID

		// 4) tag 关联。
		if err := r.replaceTags(tx, in.BrandID, profile.ID, in.TagIDs); err != nil {
			return err
		}

		// 5) audit。
		return writeLearnerOperationLog(tx, in.BrandID, in.ActorID, "learner_created", profile.ID, nil, &profile)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, in.BrandID, createdID)
}

// findOrCreateIdentity 按手机号找身份，没有则用合成 open_id 占位新建。并发下 INSERT 撞唯一约束时回查。
func (r *learnerRepository) findOrCreateIdentity(tx *gorm.DB, phone, nickname string) (int64, error) {
	var identity LearnerIdentityModel
	err := tx.Where("phone = ?", phone).First(&identity).Error
	if err == nil {
		return identity.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, apperr.ErrInternalF("查询学员身份失败", err)
	}
	p := phone
	created := LearnerIdentityModel{
		WechatOpenID: "manual:" + phone, // 合成占位；日后微信登录按手机号回填真实 open_id。
		Phone:        &p,
		Nickname:     nickname,
		Status:       "active",
	}
	if err := tx.Create(&created).Error; err != nil {
		if isUniqueViolation(err) {
			// 并发：另一会话已建同手机号身份，回查复用。
			var existing LearnerIdentityModel
			if e2 := tx.Where("phone = ?", phone).First(&existing).Error; e2 == nil {
				return existing.ID, nil
			}
		}
		return 0, apperr.ErrInternalF("创建学员身份失败", err)
	}
	return created.ID, nil
}

// findOrCreateIdentityByOpenID 按微信 openid 找身份，没有则新建（phone 留 NULL）。并发撞唯一约束回查。
// 区别于 findOrCreateIdentity（by phone + 合成 openid）——C 端微信登录的天然 key 是 wechat_open_id。
func (r *learnerRepository) findOrCreateIdentityByOpenID(tx *gorm.DB, openID, nickname string) (int64, error) {
	var identity LearnerIdentityModel
	err := tx.Where("wechat_open_id = ?", openID).First(&identity).Error
	if err == nil {
		return identity.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, apperr.ErrInternalF("查询学员身份失败", err)
	}
	created := LearnerIdentityModel{
		WechatOpenID: openID, // 真实/dev openid 作 key；phone 留 NULL（手机号绑定留 FR）。
		Nickname:     strings.TrimSpace(nickname),
		Status:       "active",
	}
	if err := tx.Create(&created).Error; err != nil {
		if isUniqueViolation(err) {
			var existing LearnerIdentityModel
			if e2 := tx.Where("wechat_open_id = ?", openID).First(&existing).Error; e2 == nil {
				return existing.ID, nil
			}
		}
		return 0, apperr.ErrInternalF("创建学员身份失败", err)
	}
	return created.ID, nil
}

// FindOrCreateProfileByOpenID 见接口注释（Batch 14a 桥接）。单事务：identity(by openid) →
// profile(by brand+identity，幂等) → 缺则 quota 门 + INSERT + audit(actor=learner)。
func (r *learnerRepository) FindOrCreateProfileByOpenID(ctx context.Context, brandID int64, openID, nickname string) (*learner.Profile, error) {
	openID = strings.TrimSpace(openID)
	if openID == "" {
		return nil, apperr.NewAppError(apperr.ErrInvalidParam, "缺少 openid", 400)
	}
	var profileID int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		identityID, ierr := r.findOrCreateIdentityByOpenID(tx, openID, nickname)
		if ierr != nil {
			return ierr
		}
		// 幂等：命中 (brand, identity) 未软删档案即返回（GORM 自动加 deleted_at IS NULL）。
		var prof BrandLearnerProfileModel
		ferr := tx.Where("brand_id = ? AND learner_identity_id = ?", brandID, identityID).First(&prof).Error
		if ferr == nil {
			profileID = prof.ID
			return nil
		}
		if !errors.Is(ferr, gorm.ErrRecordNotFound) {
			return apperr.ErrInternalF("查询学员档案失败", ferr)
		}
		// 缺则建：quota 门（max_learners 硬限）→ INSERT → audit。
		if _, _, qerr := r.guard.CheckAndCount(ctx, tx, brandID, commercial.ResourceLearner); qerr != nil {
			return qerr
		}
		created := BrandLearnerProfileModel{
			BrandID:           brandID,
			LearnerIdentityID: identityID,
			Nickname:          strings.TrimSpace(nickname),
			Status:            string(learner.StatusActive),
		}
		if cerr := tx.Create(&created).Error; cerr != nil {
			if be := profileConflictError(cerr); be != nil {
				// 并发：另一会话已建同 (brand, identity)，回查复用。
				var existing BrandLearnerProfileModel
				if e2 := tx.Where("brand_id = ? AND learner_identity_id = ?", brandID, identityID).First(&existing).Error; e2 == nil {
					profileID = existing.ID
					return nil
				}
				return be
			}
			return apperr.ErrInternalF("创建学员档案失败", cerr)
		}
		profileID = created.ID
		bID := brandID
		return audit.Write(tx, audit.Event{
			BrandID: &bID,
			Actor:   audit.Actor{Type: audit.ActorLearner, ID: created.ID},
			Action:  "learner_self_registered",
			Target:  audit.Target{Type: "brand_learner_profile", ID: created.ID},
			After:   &created,
		})
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, profileID)
}

// replaceTags 校验 tag_ids ⊆ 本 brand active 标签后硬删重插（create/update 共用）。
func (r *learnerRepository) replaceTags(tx *gorm.DB, brandID, profileID int64, tagIDs []int64) error {
	if err := tx.Where("brand_learner_profile_id = ?", profileID).
		Delete(&LearnerTagAssignmentModel{}).Error; err != nil {
		return apperr.ErrInternalF("清理学员标签失败", err)
	}
	uniq := dedupInt64(tagIDs)
	if len(uniq) == 0 {
		return nil
	}
	var validCount int64
	if err := tx.Model(&LearnerTagModel{}).
		Where("id IN ? AND brand_id = ? AND status = ?", uniq, brandID, string(learner.TagStatusActive)).
		Count(&validCount).Error; err != nil {
		return apperr.ErrInternalF("校验标签失败", err)
	}
	if int(validCount) != len(uniq) {
		return apperr.NewAppError(apperr.ErrLearnerTagNotFound, "标签不存在或已停用", 404)
	}
	rows := make([]LearnerTagAssignmentModel, len(uniq))
	for i, tid := range uniq {
		rows[i] = LearnerTagAssignmentModel{BrandID: brandID, BrandLearnerProfileID: profileID, TagID: tid}
	}
	if err := tx.Create(&rows).Error; err != nil {
		return apperr.ErrInternalF("写入学员标签失败", err)
	}
	return nil
}

func dedupInt64(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// learnerProfileRow 反范式扫描行。
type learnerProfileRow struct {
	BrandLearnerProfileModel
	Phone               string
	AvatarURL           string
	PrimaryLocationName string
}

func (r *learnerRepository) baseQuery(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).
		Table("brand_learner_profiles p").
		Select(`p.*, COALESCE(i.phone, '') AS phone, COALESCE(i.avatar_url, '') AS avatar_url, COALESCE(l.name, '') AS primary_location_name`).
		Joins("JOIN learner_identities i ON i.id = p.learner_identity_id").
		Joins("LEFT JOIN locations l ON l.id = p.primary_location_id").
		Where("p.deleted_at IS NULL")
}

func (r *learnerRepository) GetByID(ctx context.Context, brandID, id int64) (*learner.Profile, error) {
	var row learnerProfileRow
	if err := r.baseQuery(ctx).Where("p.id = ? AND p.brand_id = ?", id, brandID).Scan(&row).Error; err != nil {
		return nil, apperr.ErrInternalF("查询学员失败", err)
	}
	if row.ID == 0 {
		return nil, apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
	}
	p := toProfileDomain(&row)
	tagMap, err := r.loadTags(ctx, brandID, []int64{p.ID})
	if err != nil {
		return nil, err
	}
	if t := tagMap[p.ID]; t != nil {
		p.Tags = t
	}
	return p, nil
}

func (r *learnerRepository) List(ctx context.Context, filter learner.ListFilter, offset, limit int) ([]*learner.Profile, int64, error) {
	q := r.baseQuery(ctx).Where("p.brand_id = ?", filter.BrandID)
	if learner.IsValidStatus(filter.Status) {
		q = q.Where("p.status = ?", filter.Status)
	}
	if filter.PrimaryLocationID > 0 {
		q = q.Where("p.primary_location_id = ?", filter.PrimaryLocationID)
	}
	if s := strings.TrimSpace(filter.Query); s != "" {
		like := "%" + s + "%"
		q = q.Where("(p.nickname ILIKE ? OR i.phone ILIKE ? OR p.learner_no ILIKE ?)", like, like, like)
	}
	if filter.ScopeLocationIDs != nil {
		if len(filter.ScopeLocationIDs) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("p.primary_location_id IN ?", filter.ScopeLocationIDs)
		}
	}

	var total int64
	if err := q.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询学员列表失败", err)
	}

	var rows []learnerProfileRow
	if err := q.Order("p.id DESC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询学员列表失败", err)
	}
	items := make([]*learner.Profile, len(rows))
	ids := make([]int64, len(rows))
	for i := range rows {
		items[i] = toProfileDomain(&rows[i])
		ids[i] = items[i].ID
	}
	tagMap, err := r.loadTags(ctx, filter.BrandID, ids)
	if err != nil {
		return nil, 0, err
	}
	for _, it := range items {
		if t := tagMap[it.ID]; t != nil {
			it.Tags = t
		}
	}
	return items, total, nil
}

// loadTags 一次查回多个 profile 的标签（避 N+1，镜像 Batch 11 分类聚合）。
func (r *learnerRepository) loadTags(ctx context.Context, brandID int64, profileIDs []int64) (map[int64][]learner.TagRef, error) {
	out := map[int64][]learner.TagRef{}
	if len(profileIDs) == 0 {
		return out, nil
	}
	type tagAssignRow struct {
		ProfileID int64
		ID        int64
		Name      string
		Color     string
	}
	var rows []tagAssignRow
	if err := r.db.WithContext(ctx).
		Table("learner_tag_assignments a").
		Select("a.brand_learner_profile_id AS profile_id, t.id, t.name, COALESCE(t.color, '') AS color").
		Joins("JOIN learner_tags t ON t.id = a.tag_id").
		Where("a.brand_id = ? AND a.brand_learner_profile_id IN ?", brandID, profileIDs).
		Order("t.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询学员标签失败", err)
	}
	for _, row := range rows {
		out[row.ProfileID] = append(out[row.ProfileID], learner.TagRef{ID: row.ID, Name: row.Name, Color: row.Color})
	}
	return out, nil
}

func (r *learnerRepository) Update(ctx context.Context, brandID, actorID, id int64, in learner.UpdateInput) (*learner.Profile, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
			}
			return apperr.ErrInternalF("查询学员失败", err)
		}

		updates := map[string]interface{}{}
		if in.Nickname != nil {
			updates["nickname"] = strings.TrimSpace(*in.Nickname)
		}
		if in.PrimaryLocationID != nil {
			if *in.PrimaryLocationID <= 0 {
				updates["primary_location_id"] = nil
			} else {
				updates["primary_location_id"] = *in.PrimaryLocationID
			}
		}
		if in.LearnerNo != nil {
			updates["learner_no"] = nullableStr(*in.LearnerNo)
		}
		if in.Remark != nil {
			updates["remark"] = strings.TrimSpace(*in.Remark)
		}
		if len(updates) > 0 {
			if err := tx.Model(&BrandLearnerProfileModel{}).
				Where("id = ? AND brand_id = ?", id, brandID).Updates(updates).Error; err != nil {
				if be := profileConflictError(err); be != nil {
					return be
				}
				return apperr.ErrInternalF("更新学员失败", err)
			}
		}
		if in.TagIDs != nil {
			if err := r.replaceTags(tx, brandID, id, *in.TagIDs); err != nil {
				return err
			}
		}

		var after BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的学员失败", err)
		}
		return writeLearnerOperationLog(tx, brandID, actorID, "learner_updated", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *learnerRepository) UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*learner.Profile, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
			}
			return apperr.ErrInternalF("查询学员失败", err)
		}
		if before.Status == status {
			return nil // 幂等：状态未变。
		}
		if err := tx.Model(&BrandLearnerProfileModel{}).
			Where("id = ? AND brand_id = ?", id, brandID).Update("status", status).Error; err != nil {
			return apperr.ErrInternalF("更新学员状态失败", err)
		}
		after := before
		after.Status = status
		return writeLearnerOperationLog(tx, brandID, actorID, "learner_status_changed", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, brandID, id)
}

func (r *learnerRepository) Delete(ctx context.Context, brandID, actorID, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before BrandLearnerProfileModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
			}
			return apperr.ErrInternalF("查询学员失败", err)
		}
		refs, err := countLearnerActiveReferences(tx, brandID, id)
		if err != nil {
			return err
		}
		if refs > 0 {
			return apperr.NewAppError(apperr.ErrLearnerInUse, "学员仍有有效权益或未来预约，无法删除", 409)
		}
		res := tx.Where("id = ? AND brand_id = ?", id, brandID).Delete(&BrandLearnerProfileModel{})
		if res.Error != nil {
			return apperr.ErrInternalF("删除学员失败", res.Error)
		}
		if res.RowsAffected == 0 {
			return apperr.NewAppError(apperr.ErrLearnerNotFound, "学员不存在", 404)
		}
		return writeLearnerOperationLog(tx, brandID, actorID, "learner_deleted", id, &before, nil)
	})
}

// countLearnerActiveReferences 统计阻止删除学员的引用：active 权益 + 未结束预约。
// 13b/13c 落地前两表无数据恒 0；提前写避免返工（镜像 12a 资源 guard 提前纳入 recurring）。
func countLearnerActiveReferences(tx *gorm.DB, brandID, profileID int64) (int64, error) {
	var entCount int64
	if err := tx.Table("learner_entitlements").
		Where("brand_learner_profile_id = ? AND brand_id = ? AND status = ?", profileID, brandID, "active").
		Count(&entCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计学员权益引用失败", err)
	}
	var bookingCount int64
	if err := tx.Table("bookings").
		Where("brand_learner_profile_id = ? AND brand_id = ? AND status IN ?", profileID, brandID, []string{"booked", "pending_no_show"}).
		Count(&bookingCount).Error; err != nil {
		return 0, apperr.ErrInternalF("统计学员预约引用失败", err)
	}
	return entCount + bookingCount, nil
}

// ---- 标签 ----

func (r *learnerRepository) CreateTag(ctx context.Context, in learner.CreateTagInput) (*learner.Tag, error) {
	created := LearnerTagModel{
		BrandID: in.BrandID,
		Name:    strings.TrimSpace(in.Name),
		Color:   strings.TrimSpace(in.Color),
		Status:  string(learner.TagStatusActive),
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&created).Error; err != nil {
			if isUniqueViolation(err) {
				return apperr.NewAppError(apperr.ErrLearnerTagNameDuplicated, "标签名已存在", 409)
			}
			return apperr.ErrInternalF("创建标签失败", err)
		}
		return writeLearnerTagOperationLog(tx, in.BrandID, in.ActorID, "learner_tag_created", created.ID, nil, &created)
	})
	if err != nil {
		return nil, err
	}
	return toTagDomain(&created), nil
}

func (r *learnerRepository) ListTags(ctx context.Context, filter learner.TagListFilter) ([]*learner.Tag, error) {
	q := r.db.WithContext(ctx).Model(&LearnerTagModel{}).Where("brand_id = ?", filter.BrandID)
	if learner.IsValidTagStatus(filter.Status) {
		q = q.Where("status = ?", filter.Status)
	}
	var rows []LearnerTagModel
	if err := q.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, apperr.ErrInternalF("查询标签列表失败", err)
	}
	items := make([]*learner.Tag, len(rows))
	for i := range rows {
		items[i] = toTagDomain(&rows[i])
	}
	return items, nil
}

func (r *learnerRepository) UpdateTag(ctx context.Context, brandID, actorID, id int64, in learner.UpdateTagInput) (*learner.Tag, error) {
	var after LearnerTagModel
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var before LearnerTagModel
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&before).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.NewAppError(apperr.ErrLearnerTagNotFound, "标签不存在", 404)
			}
			return apperr.ErrInternalF("查询标签失败", err)
		}
		updates := map[string]interface{}{}
		if in.Name != nil {
			updates["name"] = strings.TrimSpace(*in.Name)
		}
		if in.Color != nil {
			updates["color"] = strings.TrimSpace(*in.Color)
		}
		if in.Status != nil {
			updates["status"] = *in.Status
		}
		if len(updates) > 0 {
			if err := tx.Model(&LearnerTagModel{}).
				Where("id = ? AND brand_id = ?", id, brandID).Updates(updates).Error; err != nil {
				if isUniqueViolation(err) {
					return apperr.NewAppError(apperr.ErrLearnerTagNameDuplicated, "标签名已存在", 409)
				}
				return apperr.ErrInternalF("更新标签失败", err)
			}
		}
		if err := tx.Where("id = ? AND brand_id = ?", id, brandID).First(&after).Error; err != nil {
			return apperr.ErrInternalF("查询更新后的标签失败", err)
		}
		return writeLearnerTagOperationLog(tx, brandID, actorID, "learner_tag_updated", id, &before, &after)
	})
	if err != nil {
		return nil, err
	}
	return toTagDomain(&after), nil
}

func writeLearnerOperationLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *BrandLearnerProfileModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "learner", ID: id},
		Before:  before,
		After:   after,
	})
}

func writeLearnerTagOperationLog(tx *gorm.DB, brandID, actorID int64, action string, id int64, before, after *LearnerTagModel) error {
	bID := brandID
	return audit.Write(tx, audit.Event{
		BrandID: &bID,
		Actor:   audit.Actor{Type: audit.ActorBrandUser, ID: actorID},
		Action:  action,
		Target:  audit.Target{Type: "learner_tag", ID: id},
		Before:  before,
		After:   after,
	})
}

func toProfileDomain(r *learnerProfileRow) *learner.Profile {
	p := &learner.Profile{
		ID:                  r.ID,
		BrandID:             r.BrandID,
		LearnerIdentityID:   r.LearnerIdentityID,
		PrimaryLocationID:   r.PrimaryLocationID,
		Nickname:            r.Nickname,
		Remark:              r.Remark,
		Status:              learner.Status(r.Status),
		CreatedAt:           r.CreatedAt,
		UpdatedAt:           r.UpdatedAt,
		Phone:               r.Phone,
		AvatarURL:           r.AvatarURL,
		PrimaryLocationName: r.PrimaryLocationName,
		Tags:                []learner.TagRef{},
	}
	if r.LearnerNo != nil {
		p.LearnerNo = *r.LearnerNo
	}
	return p
}

func toTagDomain(m *LearnerTagModel) *learner.Tag {
	return &learner.Tag{
		ID:        m.ID,
		BrandID:   m.BrandID,
		Name:      m.Name,
		Color:     m.Color,
		Status:    learner.TagStatus(m.Status),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
