package persistence

import (
	"context"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/internal/domain/brand"

	"gorm.io/gorm"
)

type brandRepository struct {
	db *gorm.DB
}

// NewBrandRepository 创建品牌仓储实现
func NewBrandRepository(db *gorm.DB) brand.Repository {
	return &brandRepository{db: db}
}

func (r *brandRepository) Create(ctx context.Context, input brand.CreateBrandInput) (*brand.Brand, error) {
	m := BrandModel{
		Name:         input.Name,
		LogoURL:      input.LogoURL,
		ContactName:  input.ContactName,
		ContactPhone: input.ContactPhone,
		Status:       string(brand.StatusPending),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.NewAppError(apperr.ErrBrandExists, "品牌已存在（联系电话重复）", 409)
		}
		return nil, apperr.ErrInternalF("创建品牌失败", err)
	}
	return toBrandDomain(&m), nil
}

func (r *brandRepository) GetByID(ctx context.Context, id int64) (*brand.Brand, error) {
	var m BrandModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌失败", err)
	}
	return toBrandDomain(&m), nil
}

func (r *brandRepository) List(ctx context.Context, offset, limit int) ([]*brand.Brand, int64, error) {
	var models []BrandModel
	var total int64

	if err := r.db.WithContext(ctx).Model(&BrandModel{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌列表失败", err)
	}

	if err := r.db.WithContext(ctx).Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌列表失败", err)
	}

	items := make([]*brand.Brand, len(models))
	for i := range models {
		items[i] = toBrandDomain(&models[i])
	}
	return items, total, nil
}

func (r *brandRepository) Update(ctx context.Context, id int64, input brand.UpdateBrandInput) (*brand.Brand, error) {
	var m BrandModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
		}
		return nil, apperr.ErrInternalF("查询品牌失败", err)
	}

	updates := make(map[string]interface{})
	if input.Name != nil {
		updates["name"] = *input.Name
	}
	if input.LogoURL != nil {
		updates["logo_url"] = *input.LogoURL
	}
	if input.ContactName != nil {
		updates["contact_name"] = *input.ContactName
	}

	if len(updates) > 0 {
		if err := r.db.WithContext(ctx).Model(&m).Updates(updates).Error; err != nil {
			return nil, apperr.ErrInternalF("更新品牌失败", err)
		}
	}

	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		return nil, apperr.ErrInternalF("查询更新后的品牌失败", err)
	}
	return toBrandDomain(&m), nil
}

func (r *brandRepository) UpdateStatus(ctx context.Context, id int64, status brand.Status) error {
	result := r.db.WithContext(ctx).Model(&BrandModel{}).Where("id = ?", id).Update("status", string(status))
	if result.Error != nil {
		return apperr.ErrInternalF("更新品牌状态失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrBrandNotFound, "品牌不存在")
	}
	return nil
}

func toBrandDomain(m *BrandModel) *brand.Brand {
	return &brand.Brand{
		ID:           m.ID,
		Name:         m.Name,
		LogoURL:      m.LogoURL,
		ContactName:  m.ContactName,
		ContactPhone: m.ContactPhone,
		Status:       brand.Status(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}
