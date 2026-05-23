package user

import "time"

// AppUser C 端用户实体
type AppUser struct {
	ID          int64     `json:"id"`
	BrandID     int64     `json:"brand_id"` // 归属品牌
	OpenID      string    `json:"openid"`
	Phone       string    `json:"phone"`
	Nickname    string    `json:"nickname"`
	AvatarURL   string    `json:"avatar_url"`
	VIPLevel    VIPLevel  `json:"vip_level"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type VIPLevel string

const (
	VIPFree  VIPLevel = "free"
	VIPPro   VIPLevel = "pro"
	VIPUltra VIPLevel = "ultra"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
)

// CreateAppUserInput 创建 C 端用户输入
type CreateAppUserInput struct {
	BrandID   int64    `validate:"required,gt=0"`
	OpenID    string   `validate:"required"`
	Phone     string   `validate:"omitempty,phone"`
	Nickname  string   `validate:"omitempty,max=50"`
	AvatarURL string   `validate:"omitempty,url"`
}
