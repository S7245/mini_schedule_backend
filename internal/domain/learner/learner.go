// Package learner 学员档案领域（brand_learner_profiles / learner_identities /
// learner_tags / learner_tag_assignments，Batch 13a）。
//
// BrandLearnerProfile 是品牌下的学员档案，关联一个跨品牌复用的 LearnerIdentity（按手机号
// find-or-create；无微信学员用合成 open_id 占位）。学员是预约闭环的前置主数据：13b 给学员
// 发权益、13c 给学员下单都绑 brand_learner_profile_id。
//
// 状态：active（正常）/ frozen（冻结，禁自助预约，不自动取消已有预约）/ inactive（停用，保留）。
package learner

import (
	"context"
	"time"
)

// Status 学员档案状态（与 DB CHECK brand_learner_profiles_status_valid 对齐）。
type Status string

const (
	StatusActive   Status = "active"
	StatusFrozen   Status = "frozen"
	StatusInactive Status = "inactive"
)

// IsValidStatus 判断输入是否合法学员状态。
func IsValidStatus(s string) bool {
	switch Status(s) {
	case StatusActive, StatusFrozen, StatusInactive:
		return true
	}
	return false
}

// TagStatus 标签状态。
type TagStatus string

const (
	TagStatusActive   TagStatus = "active"
	TagStatusInactive TagStatus = "inactive"
)

// IsValidTagStatus 判断输入是否合法标签状态。
func IsValidTagStatus(s string) bool {
	return s == string(TagStatusActive) || s == string(TagStatusInactive)
}

// TagRef 学员档案上内嵌的标签精简引用（列表/详情聚合用）。
type TagRef struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Profile 学员档案实体（含反范式 phone/avatar_url/primary_location_name + 聚合 tags）。
type Profile struct {
	ID                  int64     `json:"id"`
	BrandID             int64     `json:"brand_id"`
	LearnerIdentityID   int64     `json:"learner_identity_id"`
	PrimaryLocationID   *int64    `json:"primary_location_id"`
	LearnerNo           string    `json:"learner_no"`
	Nickname            string    `json:"nickname"`
	Remark              string    `json:"remark"`
	Status              Status    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	Phone               string    `json:"phone"`                 // 反范式（identity JOIN）。
	AvatarURL           string    `json:"avatar_url"`            // 反范式（identity JOIN）。
	PrimaryLocationName string    `json:"primary_location_name"` // 反范式（locations LEFT JOIN）。
	Tags                []TagRef  `json:"tags"`                  // 聚合，nil 规整为空切片（前端会迭代）。
}

// Tag 学员标签实体（品牌级，无 location scope）。
type Tag struct {
	ID        int64     `json:"id"`
	BrandID   int64     `json:"brand_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Status    TagStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateInput 创建学员入参。
type CreateInput struct {
	BrandID           int64
	ActorID           int64
	Phone             string
	Nickname          string
	PrimaryLocationID *int64
	LearnerNo         string
	Remark            string
	TagIDs            []int64
}

// UpdateInput 编辑学员入参（白名单）。
//   - 指针字段 nil = 不改。
//   - PrimaryLocationID：nil 不改；指向 0 = 清空为 NULL；>0 = 设为该门店。
//   - TagIDs：nil 不改；非 nil（含空切片）= 全量替换。
//   - Status 不在此，走 UpdateStatus（freeze 权限门）。
type UpdateInput struct {
	Nickname          *string
	PrimaryLocationID *int64
	LearnerNo         *string
	Remark            *string
	TagIDs            *[]int64
}

// ListFilter 列表查询。零值不过滤；ScopeLocationIDs 非 nil 时按 data_scope 收紧。
type ListFilter struct {
	BrandID           int64
	Status            string
	PrimaryLocationID int64
	Query             string // 模糊匹配 nickname / phone / learner_no。
	ScopeLocationIDs  []int64
}

// CreateTagInput 创建标签入参。
type CreateTagInput struct {
	BrandID int64
	ActorID int64
	Name    string
	Color   string
}

// UpdateTagInput 编辑标签入参（白名单）。
type UpdateTagInput struct {
	Name   *string
	Color  *string
	Status *string
}

// TagListFilter 标签列表查询。
type TagListFilter struct {
	BrandID int64
	Status  string
}

// Repository 学员档案仓储接口。
type Repository interface {
	// Create 在单事务内：quota 校验（SubscriptionGuard.ResourceLearner）→ find-or-create
	// identity by phone → INSERT profile（重复手机号→LEARNER_ALREADY_EXISTS；重复学号→
	// LEARNER_NO_DUPLICATED）→ tag 关联（tag_ids 非本 brand→LEARNER_TAG_NOT_FOUND）→ audit。
	Create(ctx context.Context, in CreateInput) (*Profile, error)
	// FindOrCreateProfileByOpenID C 端微信登录桥接（Batch 14a）：find-or-create identity by
	// wechat_open_id（phone 留 NULL，手机号绑定留 FR）+ profile by (brand, identity)。幂等——每次
	// 登录可重入，命中即返回。缺 profile 才建（quota 门 + audit actor=learner）。
	FindOrCreateProfileByOpenID(ctx context.Context, brandID int64, openID, nickname string) (*Profile, error)
	GetByID(ctx context.Context, brandID, id int64) (*Profile, error)
	List(ctx context.Context, filter ListFilter, offset, limit int) ([]*Profile, int64, error)
	Update(ctx context.Context, brandID, actorID, id int64, in UpdateInput) (*Profile, error)
	UpdateStatus(ctx context.Context, brandID, actorID, id int64, status string) (*Profile, error)
	// Delete 软删；被 active 权益或未来预约引用 → LEARNER_IN_USE（13b/13c 落地前恒 0）。
	Delete(ctx context.Context, brandID, actorID, id int64) error

	CreateTag(ctx context.Context, in CreateTagInput) (*Tag, error)
	ListTags(ctx context.Context, filter TagListFilter) ([]*Tag, error)
	UpdateTag(ctx context.Context, brandID, actorID, id int64, in UpdateTagInput) (*Tag, error)
}
