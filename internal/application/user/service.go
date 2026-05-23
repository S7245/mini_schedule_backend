package user

import (
	"context"
	"log/slog"

	"golang.org/x/crypto/bcrypt"

	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/internal/domain/user"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/cache"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
)

// AppUserService C 端用户应用服务
type AppUserService struct {
	repo user.AppUserRepository
	cfg  *config.Config
}

func NewAppUserService(repo user.AppUserRepository, cfg *config.Config) *AppUserService {
	return &AppUserService{repo: repo, cfg: cfg}
}

func (s *AppUserService) CreateUser(ctx context.Context, input user.CreateAppUserInput) (*user.AppUser, error) {
	return s.repo.Create(ctx, input)
}

func (s *AppUserService) GetUser(ctx context.Context, id int64) (*user.AppUser, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AppUserService) UpdateUser(ctx context.Context, id int64, nickname, avatarURL string) (*user.AppUser, error) {
	return s.repo.Update(ctx, id, nickname, avatarURL)
}

func (s *AppUserService) GetUserByOpenID(ctx context.Context, brandID int64, openID string) (*user.AppUser, error) {
	return s.repo.GetByBrandIDAndOpenID(ctx, brandID, openID)
}

func (s *AppUserService) GetOrCreateByOpenID(ctx context.Context, brandID int64, openID, nickname string) (*user.AppUser, error) {
	u, err := s.repo.GetByBrandIDAndOpenID(ctx, brandID, openID)
	if err == nil {
		return u, nil
	}
	// 用户不存在，创建新用户
	return s.repo.Create(ctx, user.CreateAppUserInput{
		BrandID:  brandID,
		OpenID:   openID,
		Nickname: nickname,
	})
}

func (s *AppUserService) ListUsersByBrand(ctx context.Context, brandID int64, page, pageSize int) ([]*user.AppUser, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = s.cfg.Pagination.DefaultPageSize
	}
	if pageSize > s.cfg.Pagination.MaxPageSize {
		pageSize = s.cfg.Pagination.MaxPageSize
	}

	offset := (page - 1) * pageSize
	return s.repo.ListByBrandID(ctx, brandID, offset, pageSize)
}

// BrandUserService 品牌管理员应用服务
type BrandUserService struct {
	repo   user.BrandUserRepository
	jwtSvc *cache.Service
	log    *slog.Logger
	cfg    *config.Config
}

func NewBrandUserService(repo user.BrandUserRepository, jwtSvc *cache.Service, log *slog.Logger, cfg *config.Config) *BrandUserService {
	return &BrandUserService{repo: repo, jwtSvc: jwtSvc, log: log, cfg: cfg}
}

func (s *BrandUserService) CreateUser(ctx context.Context, input user.CreateBrandUserInput) (*user.BrandUser, error) {
	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.ErrInternalF("密码加密失败", err)
	}
	input.Password = string(hashedPassword)

	return s.repo.Create(ctx, input)
}

func (s *BrandUserService) GetUserByPhone(ctx context.Context, phone string) (*user.BrandUser, error) {
	return s.repo.GetByPhone(ctx, phone)
}

func (s *BrandUserService) Login(ctx context.Context, input user.LoginBrandUserInput) (string, string, error) {
	u, err := s.repo.GetByPhone(ctx, input.Phone)
	if err != nil {
		return "", "", apperr.ErrUnauthorizedF("手机号或密码错误")
	}

	if u.Status == user.StatusDisabled {
		return "", "", apperr.NewAppError(apperr.ErrAccountDisabled, "账号已被禁用", 403)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(input.Password)); err != nil {
		return "", "", apperr.ErrUnauthorizedF("手机号或密码错误")
	}

	payload := cache.TokenPayload{
		UserID:   u.ID,
		BrandID:  u.BrandID,
		UserType: "brand",
	}

	accessToken, err := s.jwtSvc.GenerateToken(payload)
	if err != nil {
		return "", "", apperr.ErrInternalF("生成令牌失败", err)
	}

	refreshToken, err := s.jwtSvc.GenerateRefreshToken(payload)
	if err != nil {
		return "", "", apperr.ErrInternalF("生成刷新令牌失败", err)
	}

	return accessToken, refreshToken, nil
}

// AdminUserService 平台管理员应用服务
type AdminUserService struct {
	repo   user.AdminUserRepository
	jwtSvc *cache.Service
	log    *slog.Logger
	cfg    *config.Config
}

func NewAdminUserService(repo user.AdminUserRepository, jwtSvc *cache.Service, log *slog.Logger, cfg *config.Config) *AdminUserService {
	return &AdminUserService{repo: repo, jwtSvc: jwtSvc, log: log, cfg: cfg}
}

func (s *AdminUserService) CreateUser(ctx context.Context, input user.CreateAdminUserInput) (*user.AdminUser, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.ErrInternalF("密码加密失败", err)
	}
	input.Password = string(hashedPassword)

	return s.repo.Create(ctx, input)
}

func (s *AdminUserService) GetUserByUsername(ctx context.Context, username string) (*user.AdminUser, error) {
	return s.repo.GetByUsername(ctx, username)
}

func (s *AdminUserService) Login(ctx context.Context, input user.LoginAdminUserInput) (string, string, error) {
	u, err := s.repo.GetByUsername(ctx, input.Username)
	if err != nil {
		return "", "", apperr.ErrUnauthorizedF("用户名或密码错误")
	}

	if u.Status == user.StatusDisabled {
		return "", "", apperr.NewAppError(apperr.ErrAccountDisabled, "账号已被禁用", 403)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(input.Password)); err != nil {
		return "", "", apperr.ErrUnauthorizedF("用户名或密码错误")
	}

	payload := cache.TokenPayload{
		UserID:   u.ID,
		UserType: "admin",
	}

	accessToken, err := s.jwtSvc.GenerateToken(payload)
	if err != nil {
		return "", "", apperr.ErrInternalF("生成令牌失败", err)
	}

	refreshToken, err := s.jwtSvc.GenerateRefreshToken(payload)
	if err != nil {
		return "", "", apperr.ErrInternalF("生成刷新令牌失败", err)
	}

	return accessToken, refreshToken, nil
}

func (s *AdminUserService) ListUsers(ctx context.Context, page, pageSize int) ([]*user.AdminUser, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = s.cfg.Pagination.DefaultPageSize
	}
	if pageSize > s.cfg.Pagination.MaxPageSize {
		pageSize = s.cfg.Pagination.MaxPageSize
	}

	offset := (page - 1) * pageSize
	return s.repo.List(ctx, offset, pageSize)
}
