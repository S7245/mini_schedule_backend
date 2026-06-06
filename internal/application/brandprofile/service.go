package brandprofile

import (
	"context"
	"regexp"
	"strings"

	"github.com/zkw/mini-schedule/backend/internal/infrastructure/persistence"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

// Service brand profile 应用服务（向导第 1 步专用）。
type Service struct {
	repo persistence.BrandProfileRepository
}

// NewService 创建 service。
func NewService(repo persistence.BrandProfileRepository) *Service {
	return &Service{repo: repo}
}

// Input 是 PATCH /brand/profile 入参（白名单）。
type Input struct {
	LogoURL      *string
	Description  *string
	IndustryType *string
	BrandCode    *string
	ContactEmail *string
}

// 简单 email 正则；validator 包做 input binding 时也会处理，service 层 defensive 兜底。
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const (
	maxDescriptionLen  = 2000
	maxBrandCodeLen    = 50
	maxIndustryTypeLen = 50
	maxLogoURLLen      = 500
	maxContactEmailLen = 100
)

// GetProfile 透传 GET。
func (s *Service) GetProfile(ctx context.Context, brandID int64) (*persistence.BrandProfile, error) {
	if brandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}
	return s.repo.GetProfile(ctx, brandID)
}

// UpdateProfile 校验输入 → 写库。
//
// - description 超长 / industry_type 超长 / logo_url 超长 → INVALID_PARAM
// - contact_email 非邮箱格式 → INVALID_PARAM
// - brand_code 长度 > 50 → INVALID_PARAM；空字符串视为清除
// - name / contact_phone / contact_name 任何场景下都不会被 UPDATE（白名单已隔离）
func (s *Service) UpdateProfile(ctx context.Context, brandID int64, in Input) (*persistence.BrandProfile, error) {
	if brandID <= 0 {
		return nil, apperr.ErrBadRequest("品牌 ID 无效")
	}

	if in.Description != nil {
		v := strings.TrimSpace(*in.Description)
		if len([]rune(v)) > maxDescriptionLen {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "description 长度超出限制", 400)
		}
		in.Description = &v
	}
	if in.IndustryType != nil {
		v := strings.TrimSpace(*in.IndustryType)
		if len(v) > maxIndustryTypeLen {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "industry_type 长度超出限制", 400)
		}
		in.IndustryType = &v
	}
	if in.LogoURL != nil {
		v := strings.TrimSpace(*in.LogoURL)
		if len(v) > maxLogoURLLen {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "logo_url 长度超出限制", 400)
		}
		in.LogoURL = &v
	}
	if in.BrandCode != nil {
		v := strings.TrimSpace(*in.BrandCode)
		if len(v) > maxBrandCodeLen {
			return nil, apperr.NewAppError(apperr.ErrInvalidParam, "brand_code 长度超出限制", 400)
		}
		in.BrandCode = &v
	}
	if in.ContactEmail != nil {
		v := strings.TrimSpace(*in.ContactEmail)
		if v != "" {
			if len(v) > maxContactEmailLen {
				return nil, apperr.NewAppError(apperr.ErrInvalidParam, "contact_email 长度超出限制", 400)
			}
			if !emailPattern.MatchString(v) {
				return nil, apperr.NewAppError(apperr.ErrInvalidParam, "contact_email 格式错误", 400)
			}
		}
		in.ContactEmail = &v
	}

	return s.repo.UpdateProfile(ctx, brandID, persistence.UpdateBrandProfileInput{
		LogoURL:      in.LogoURL,
		Description:  in.Description,
		IndustryType: in.IndustryType,
		BrandCode:    in.BrandCode,
		ContactEmail: in.ContactEmail,
	})
}
