package brand

import "time"

// Brand 品牌实体（聚合根）
type Brand struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	LogoURL     string    `json:"logo_url"`
	ContactName string    `json:"contact_name"`
	ContactPhone string   `json:"contact_phone"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Status 品牌状态
type Status string

const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusPending  Status = "pending" // 待审核
)

// CreateBrandInput 创建品牌输入
type CreateBrandInput struct {
	Name         string `validate:"required,min=2,max=100"`
	LogoURL      string `validate:"omitempty,url"`
	ContactName  string `validate:"required,min=2,max=50"`
	ContactPhone string `validate:"required,phone"`
}

// UpdateBrandInput 更新品牌输入
type UpdateBrandInput struct {
	Name        *string `validate:"omitempty,min=2,max=100"`
	LogoURL     *string `validate:"omitempty,url"`
	ContactName *string `validate:"omitempty,min=2,max=50"`
}
