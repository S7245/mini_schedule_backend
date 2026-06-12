package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/zkw/mini-schedule/backend/internal/domain/user"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

type appUserRepository struct {
	db *gorm.DB
}

// NewAppUserRepository 创建 C 端用户仓储实现
func NewAppUserRepository(db *gorm.DB) user.AppUserRepository {
	return &appUserRepository{db: db}
}

func (r *appUserRepository) Create(ctx context.Context, input user.CreateAppUserInput) (*user.AppUser, error) {
	m := AppUserModel{
		BrandID:   input.BrandID,
		OpenID:    input.OpenID,
		Phone:     input.Phone,
		Nickname:  input.Nickname,
		AvatarURL: input.AvatarURL,
		VIPLevel:  string(user.VIPFree),
		Status:    string(user.StatusActive),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.NewAppError(apperr.ErrUserExists, "用户已存在（OpenID 重复）", 409)
		}
		return nil, apperr.ErrInternalF("创建用户失败", err)
	}
	return toAppUserDomain(&m), nil
}

func (r *appUserRepository) GetByID(ctx context.Context, id int64) (*user.AppUser, error) {
	var m AppUserModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
		}
		return nil, apperr.ErrInternalF("查询用户失败", err)
	}
	return toAppUserDomain(&m), nil
}

func (r *appUserRepository) GetByBrandIDAndOpenID(ctx context.Context, brandID int64, openID string) (*user.AppUser, error) {
	var m AppUserModel
	if err := r.db.WithContext(ctx).Where("brand_id = ? AND open_id = ?", brandID, openID).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
		}
		return nil, apperr.ErrInternalF("查询用户失败", err)
	}
	return toAppUserDomain(&m), nil
}

func (r *appUserRepository) ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*user.AppUser, int64, error) {
	var models []AppUserModel
	var total int64

	query := r.db.WithContext(ctx).Model(&AppUserModel{}).Where("brand_id = ?", brandID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询用户列表失败", err)
	}

	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询用户列表失败", err)
	}

	items := make([]*user.AppUser, len(models))
	for i := range models {
		items[i] = toAppUserDomain(&models[i])
	}
	return items, total, nil
}

func (r *appUserRepository) Update(ctx context.Context, id int64, nickname, avatarURL string) (*user.AppUser, error) {
	updates := make(map[string]interface{})
	if nickname != "" {
		updates["nickname"] = nickname
	}
	if avatarURL != "" {
		updates["avatar_url"] = avatarURL
	}

	if len(updates) == 0 {
		return r.GetByID(ctx, id)
	}

	result := r.db.WithContext(ctx).Model(&AppUserModel{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return nil, apperr.ErrInternalF("更新用户失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
	}
	return r.GetByID(ctx, id)
}

func (r *appUserRepository) UpdateVIPLevel(ctx context.Context, id int64, level user.VIPLevel) error {
	result := r.db.WithContext(ctx).Model(&AppUserModel{}).Where("id = ?", id).Update("vip_level", string(level))
	if result.Error != nil {
		return apperr.ErrInternalF("更新 VIP 等级失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
	}
	return nil
}

func toAppUserDomain(m *AppUserModel) *user.AppUser {
	return &user.AppUser{
		ID:        m.ID,
		BrandID:   m.BrandID,
		OpenID:    m.OpenID,
		Phone:     m.Phone,
		Nickname:  m.Nickname,
		AvatarURL: m.AvatarURL,
		VIPLevel:  user.VIPLevel(m.VIPLevel),
		Status:    user.Status(m.Status),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

type brandUserRepository struct {
	db *gorm.DB
}

// NewBrandUserRepository 创建品牌管理员仓储实现
func NewBrandUserRepository(db *gorm.DB) user.BrandUserRepository {
	return &brandUserRepository{db: db}
}

func (r *brandUserRepository) Create(ctx context.Context, input user.CreateBrandUserInput) (*user.BrandUser, error) {
	m := BrandUserModel{
		BrandID:      input.BrandID,
		Phone:        input.Phone,
		PasswordHash: input.Password, // 注意：实际应在 application 层 hash
		Name:         input.Name,
		Status:       string(user.StatusActive),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.NewAppError(apperr.ErrUserExists, "手机号已注册", 409)
		}
		return nil, apperr.ErrInternalF("创建品牌管理员失败", err)
	}
	return toBrandUserDomain(&m), nil
}

func (r *brandUserRepository) GetByID(ctx context.Context, id int64) (*user.BrandUser, error) {
	var m BrandUserModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
		}
		return nil, apperr.ErrInternalF("查询用户失败", err)
	}
	return toBrandUserDomain(&m), nil
}

func (r *brandUserRepository) GetByPhone(ctx context.Context, phone string) (*user.BrandUser, error) {
	var m BrandUserModel
	if err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
		}
		return nil, apperr.ErrInternalF("查询用户失败", err)
	}
	return toBrandUserDomain(&m), nil
}

func (r *brandUserRepository) ListByBrandID(ctx context.Context, brandID int64, offset, limit int) ([]*user.BrandUser, int64, error) {
	var models []BrandUserModel
	var total int64

	query := r.db.WithContext(ctx).Model(&BrandUserModel{}).Where("brand_id = ?", brandID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌管理员列表失败", err)
	}

	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询品牌管理员列表失败", err)
	}

	items := make([]*user.BrandUser, len(models))
	for i := range models {
		items[i] = toBrandUserDomain(&models[i])
	}
	return items, total, nil
}

func (r *brandUserRepository) UpdateStatus(ctx context.Context, id int64, status user.Status) error {
	result := r.db.WithContext(ctx).Model(&BrandUserModel{}).Where("id = ?", id).Update("status", string(status))
	if result.Error != nil {
		return apperr.ErrInternalF("更新用户状态失败", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperr.ErrNotFoundF(apperr.ErrUserNotFound, "用户不存在")
	}
	return nil
}

func toBrandUserDomain(m *BrandUserModel) *user.BrandUser {
	return &user.BrandUser{
		ID:           m.ID,
		BrandID:      m.BrandID,
		Phone:        m.Phone,
		PasswordHash: m.PasswordHash,
		Name:         m.Name,
		Status:       user.Status(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

type adminUserRepository struct {
	db *gorm.DB
}

// NewAdminUserRepository 创建平台管理员仓储实现
func NewAdminUserRepository(db *gorm.DB) user.AdminUserRepository {
	return &adminUserRepository{db: db}
}

func (r *adminUserRepository) Create(ctx context.Context, input user.CreateAdminUserInput) (*user.AdminUser, error) {
	m := AdminUserModel{
		Username:     input.Username,
		PasswordHash: input.Password, // 注意：实际应在 application 层 hash
		Role:         string(input.Role),
		Status:       string(user.StatusActive),
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, apperr.NewAppError(apperr.ErrUserExists, "用户名已存在", 409)
		}
		return nil, apperr.ErrInternalF("创建管理员失败", err)
	}
	return toAdminUserDomain(&m), nil
}

func (r *adminUserRepository) GetByID(ctx context.Context, id int64) (*user.AdminUser, error) {
	var m AdminUserModel
	if err := r.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "管理员不存在")
		}
		return nil, apperr.ErrInternalF("查询管理员失败", err)
	}
	return toAdminUserDomain(&m), nil
}

func (r *adminUserRepository) GetByUsername(ctx context.Context, username string) (*user.AdminUser, error) {
	var m AdminUserModel
	if err := r.db.WithContext(ctx).Where("username = ?", username).First(&m).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperr.ErrNotFoundF(apperr.ErrUserNotFound, "管理员不存在")
		}
		return nil, apperr.ErrInternalF("查询管理员失败", err)
	}
	return toAdminUserDomain(&m), nil
}

func (r *adminUserRepository) List(ctx context.Context, offset, limit int) ([]*user.AdminUser, int64, error) {
	var models []AdminUserModel
	var total int64

	if err := r.db.WithContext(ctx).Model(&AdminUserModel{}).Count(&total).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询管理员列表失败", err)
	}

	if err := r.db.WithContext(ctx).Order("id DESC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, apperr.ErrInternalF("查询管理员列表失败", err)
	}

	items := make([]*user.AdminUser, len(models))
	for i := range models {
		items[i] = toAdminUserDomain(&models[i])
	}
	return items, total, nil
}

func toAdminUserDomain(m *AdminUserModel) *user.AdminUser {
	return &user.AdminUser{
		ID:           m.ID,
		Username:     m.Username,
		PasswordHash: m.PasswordHash,
		Role:         user.Role(m.Role),
		Status:       user.Status(m.Status),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// isUniqueViolation 判断是否为 PostgreSQL 唯一约束冲突。
// 首选 pgconn.PgError code 23505（pgx 驱动下最可靠）；旧的错误字符串匹配作为兜底，
// 但 pgx 的 message 形如 `... (SQLSTATE 23505)`，既无 "ERROR: " 前缀也不以 "duplicates"
// 结尾，纯字符串前缀判断会漏判（曾导致手机号重复时返 500 而非业务错误），故必须走 code。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	// 兜底：极少数未包装成 pgconn.PgError 的路径，按 message 关键字宽松匹配。
	msg := err.Error()
	return strings.Contains(msg, "SQLSTATE 23505") ||
		strings.Contains(msg, "duplicate key value") ||
		strings.HasSuffix(msg, "duplicates")
}
