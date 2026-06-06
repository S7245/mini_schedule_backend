package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// BrandProfile 是 GET/PATCH /brand/profile 的输出 DTO。
type BrandProfile struct {
	ID                    int64      `json:"id"`
	Name                  string     `json:"name"`
	LogoURL               string     `json:"logo_url"`
	ContactName           string     `json:"contact_name"`
	ContactPhone          string     `json:"contact_phone"`
	ContactEmail          string     `json:"contact_email"`
	BrandCode             string     `json:"brand_code"`
	IndustryType          string     `json:"industry_type"`
	Description           string     `json:"description"`
	Status                string     `json:"status"`
	OnboardingStatus      string     `json:"onboarding_status"`
	OnboardingCompletedAt *time.Time `json:"onboarding_completed_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// UpdateBrandProfileInput 是白名单 PATCH 输入；name/contact_phone/contact_name 不接受。
type UpdateBrandProfileInput struct {
	LogoURL      *string
	Description  *string
	IndustryType *string
	BrandCode    *string
	ContactEmail *string
}

// BrandProfileRepository 是 brand profile 单独的仓储接口。
//
// 拆出来一个独立 repo，避免和 domain/brand 的 Repository 杂糅 — 后者只关心
// platform admin 视角下的 brand 列表。profile 是 brand 端自己看 / 改的部分。
type BrandProfileRepository interface {
	GetProfile(ctx context.Context, brandID int64) (*BrandProfile, error)
	UpdateProfile(ctx context.Context, brandID int64, input UpdateBrandProfileInput) (*BrandProfile, error)
}

type brandProfileRepository struct {
	db *gorm.DB
}

// NewBrandProfileRepository 创建 brand profile 仓储。
func NewBrandProfileRepository(db *gorm.DB) BrandProfileRepository {
	return &brandProfileRepository{db: db}
}

func (r *brandProfileRepository) GetProfile(ctx context.Context, brandID int64) (*BrandProfile, error) {
	var m BrandModel
	if err := r.db.WithContext(ctx).Where("id = ?", brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌资料失败", err)
	}
	return brandModelToProfile(&m), nil
}

func (r *brandProfileRepository) UpdateProfile(ctx context.Context, brandID int64, input UpdateBrandProfileInput) (*BrandProfile, error) {
	var m BrandModel
	if err := r.db.WithContext(ctx).Where("id = ?", brandID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌失败", err)
	}

	updates := map[string]interface{}{}
	if input.LogoURL != nil {
		updates["logo_url"] = strings.TrimSpace(*input.LogoURL)
	}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
	}
	if input.IndustryType != nil {
		updates["industry_type"] = strings.TrimSpace(*input.IndustryType)
	}
	if input.BrandCode != nil {
		code := strings.TrimSpace(*input.BrandCode)
		if code == "" {
			updates["brand_code"] = nil
		} else {
			updates["brand_code"] = code
		}
	}
	if input.ContactEmail != nil {
		updates["contact_email"] = strings.TrimSpace(*input.ContactEmail)
	}

	if len(updates) > 0 {
		if err := r.db.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
			if isUniqueViolation(err) {
				return nil, apperr.NewAppError(apperr.ErrBrandCodeDuplicated, "品牌编码已被占用", 409)
			}
			return nil, apperr.ErrInternalF("更新品牌资料失败", err)
		}
	}

	if err := r.db.WithContext(ctx).Where("id = ?", brandID).First(&m).Error; err != nil {
		return nil, apperr.ErrInternalF("查询更新后的品牌失败", err)
	}
	return brandModelToProfile(&m), nil
}

func brandModelToProfile(m *BrandModel) *BrandProfile {
	return &BrandProfile{
		ID:                    m.ID,
		Name:                  m.Name,
		LogoURL:               m.LogoURL,
		ContactName:           m.ContactName,
		ContactPhone:          m.ContactPhone,
		ContactEmail:          m.ContactEmail,
		BrandCode:             m.BrandCode,
		IndustryType:          m.IndustryType,
		Description:           m.Description,
		Status:                m.Status,
		OnboardingStatus:      m.OnboardingStatus,
		OnboardingCompletedAt: m.OnboardingCompletedAt,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
}
