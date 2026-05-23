package user

import "time"

// BrandUser 品牌管理员实体
type BrandUser struct {
	ID           int64     `json:"id"`
	BrandID      int64     `json:"brand_id"`
	Phone        string    `json:"phone"`
	PasswordHash string    `json:"-"` // 不暴露给前端
	Name         string    `json:"name"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateBrandUserInput 创建品牌管理员输入
type CreateBrandUserInput struct {
	BrandID  int64  `validate:"required,gt=0"`
	Phone    string `validate:"required,phone"`
	Password string `validate:"required,min=6,max=64"`
	Name     string `validate:"required,min=2,max=50"`
}

// LoginBrandUserInput 品牌管理员登录输入
type LoginBrandUserInput struct {
	Phone    string `validate:"required,phone"`
	Password string `validate:"required"`
}
