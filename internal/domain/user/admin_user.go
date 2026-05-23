package user

import "time"

// AdminUser 平台管理员实体
type AdminUser struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	Status       Status    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Role string

const (
	RoleSuperAdmin Role = "super_admin"
	RoleOperator   Role = "operator"
	RoleSupport    Role = "support"
)

// CreateAdminUserInput 创建平台管理员输入
type CreateAdminUserInput struct {
	Username string `validate:"required,min=3,max=50"`
	Password string `validate:"required,min=6,max=64"`
	Role     Role   `validate:"required,oneof=super_admin operator support"`
}

// LoginAdminUserInput 平台管理员登录输入
type LoginAdminUserInput struct {
	Username string `validate:"required"`
	Password string `validate:"required"`
}
