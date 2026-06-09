package persistence

import "time"

// PermissionModel permissions 表（系统级，不与 brand 关联）。
type PermissionModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Code        string `gorm:"size:80;not null;uniqueIndex"`
	Domain      string `gorm:"size:50;not null"`
	Action      string `gorm:"size:50;not null"`
	Name        string `gorm:"size:100;not null"`
	Description string `gorm:"size:500"`
	Status      string `gorm:"size:20;not null;default:active"`
}

func (PermissionModel) TableName() string { return "permissions" }

// RoleTemplateModel role_templates 表（注册流程 / backfill 时复制到 brand_roles）。
type RoleTemplateModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Code        string `gorm:"size:80;not null;uniqueIndex"`
	Name        string `gorm:"size:100;not null"`
	ScopeType   string `gorm:"size:20;not null"`
	Description string `gorm:"size:500"`
	Status      string `gorm:"size:20;not null;default:active"`
}

func (RoleTemplateModel) TableName() string { return "role_templates" }

// RoleTemplatePermissionModel 模板 ↔ 权限映射。
type RoleTemplatePermissionModel struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt    time.Time
	TemplateID   int64 `gorm:"not null;index"`
	PermissionID int64 `gorm:"not null;index"`
}

func (RoleTemplatePermissionModel) TableName() string { return "role_template_permissions" }

// BrandRoleModel brand_roles 表（注册时复制自 role_templates）。
type BrandRoleModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	BrandID     int64  `gorm:"not null;index"`
	TemplateID  *int64 `gorm:"index"`
	Code        string `gorm:"size:80;not null"`
	Name        string `gorm:"size:100;not null"`
	ScopeType   string `gorm:"size:20;not null"`
	IsSystem    bool   `gorm:"not null;default:true"`
	Status      string `gorm:"size:20;not null;default:active"`
	Description string `gorm:"size:500"`
}

func (BrandRoleModel) TableName() string { return "brand_roles" }

// BrandRolePermissionModel brand_roles ↔ permissions 关联。
type BrandRolePermissionModel struct {
	ID           int64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt    time.Time
	BrandID      int64 `gorm:"not null;index"`
	RoleID       int64 `gorm:"not null;index"`
	PermissionID int64 `gorm:"not null;index"`
}

func (BrandRolePermissionModel) TableName() string { return "brand_role_permissions" }

// BrandUserRoleAssignmentModel brand_user 与 brand_role 的关联。
type BrandUserRoleAssignmentModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	BrandID     int64  `gorm:"not null;index"`
	BrandUserID int64  `gorm:"not null;index"`
	RoleID      int64  `gorm:"not null;index"`
	LocationID  *int64 `gorm:"index"`
	DataScope   string `gorm:"size:30;not null;default:role_default"`
	Status      string `gorm:"size:20;not null;default:active"`
}

func (BrandUserRoleAssignmentModel) TableName() string { return "brand_user_role_assignments" }

// StaffLocationAssignmentModel staff_location_assignments 表。
type StaffLocationAssignmentModel struct {
	ID             int64 `gorm:"primaryKey;autoIncrement"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	BrandID        int64  `gorm:"not null;index"`
	BrandUserID    int64  `gorm:"not null;index"`
	LocationID     int64  `gorm:"not null;index"`
	AssignmentType string `gorm:"size:30;not null;default:member"`
	IsPrimary      bool   `gorm:"not null;default:false"`
	Status         string `gorm:"size:20;not null;default:active"`
}

func (StaffLocationAssignmentModel) TableName() string { return "staff_location_assignments" }
