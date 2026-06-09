// Package instructor 教练档案领域（与 brand_user 一对一）。
package instructor

import (
	"context"
	"time"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// IsValidStatus 校验状态字符串。
func IsValidStatus(s string) bool {
	return s == string(StatusActive) || s == string(StatusInactive)
}

// Profile instructor_profiles 表的领域投影。
type Profile struct {
	ID                  int64     `json:"id"`
	BrandID             int64     `json:"brand_id"`
	BrandUserID         int64     `json:"brand_user_id"`
	DisplayName         string    `json:"display_name"`
	AvatarURL           string    `json:"avatar_url,omitempty"`
	Bio                 string    `json:"bio,omitempty"`
	Specialties         string    `json:"specialties,omitempty"`
	Certificates        string    `json:"certificates,omitempty"`
	IsVisibleToLearners bool      `json:"is_visible_to_learners"`
	IsSchedulable       bool      `json:"is_schedulable"`
	Status              Status    `json:"status"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// UpsertInput PUT /staff/:id/instructor 入参。
type UpsertInput struct {
	BrandID             int64
	BrandUserID         int64
	DisplayName         string
	AvatarURL           string
	Bio                 string
	Specialties         string
	Certificates        string
	IsVisibleToLearners bool
	IsSchedulable       bool
	Status              Status
}

// Repository 教练档案仓储接口。
type Repository interface {
	GetByBrandUserID(ctx context.Context, brandID, brandUserID int64) (*Profile, error)
	Upsert(ctx context.Context, actorID int64, in UpsertInput) (*Profile, error)
	Delete(ctx context.Context, brandID, actorID, brandUserID int64) error
}
