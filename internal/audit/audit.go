// Package audit 提供统一的 OperationLog 写入入口。
//
// 设计原则：
//   - 不引 internal/infrastructure/persistence（避免循环依赖）；
//     直接用 tx.Table("operation_logs").Create(map[string]interface{}{...})。
//   - ActorType 走显式枚举校验，防止 actor_type 字段散漂移。
//   - Before / After 自动 json.Marshal 到 metadata。
//
// 该包是 Batch 4 FR "OperationLog 合并 audit pkg" 的落地实现。
// Batch 5 起 Location / Staff / Instructor / Commercial 等所有需要写 OperationLog
// 的入口都应该调 audit.Write，禁止再直接 tx.Create(&persistence.OperationLogModel{}).
package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ActorType 操作者类型枚举。
type ActorType string

const (
	ActorBrandUser     ActorType = "brand_user"
	ActorPlatformAdmin ActorType = "platform_admin"
	ActorSystem        ActorType = "system"
	// ActorLearner C 端学员自助操作（Batch 14a）。DB CHECK operation_logs_actor_type_valid 已含 'learner'。
	ActorLearner ActorType = "learner"
)

// IsValidActorType 校验枚举值。
func IsValidActorType(t ActorType) bool {
	switch t {
	case ActorBrandUser, ActorPlatformAdmin, ActorSystem, ActorLearner:
		return true
	}
	return false
}

// Actor 操作者。ID 为 0 / nil 表示未知或系统操作。
type Actor struct {
	Type ActorType
	ID   int64
}

// Target 操作对象。Type 是字符串（如 "location" / "staff" / "instructor_profile" / "saas_plan_order"）。
type Target struct {
	Type string
	ID   int64
}

// Event 一次操作日志条目的输入。
//
// BrandID 用指针为允许 nil（平台级操作）。
// Before / After 任意可 json.Marshal 类型；都为 nil 时 metadata 写 "{}"。
// Reason 是可选的原因摘要（已存进 operation_logs.reason 列）。
type Event struct {
	BrandID *int64
	Actor   Actor
	Action  string
	Target  Target
	Reason  string
	Before  any
	After   any
}

// Write 在给定事务里写一条 operation_logs。
//
// 调用方负责：把 tx 是当前事务（不是 db）传进来；保证 Action 非空。
// 返回的 error 用于让调用方决定是否回滚事务。
func Write(tx *gorm.DB, e Event) error {
	if tx == nil {
		return errors.New("audit: nil tx")
	}
	if e.Action == "" {
		return errors.New("audit: empty action")
	}
	if !IsValidActorType(e.Actor.Type) {
		return fmt.Errorf("audit: invalid actor_type %q", e.Actor.Type)
	}

	meta, err := json.Marshal(map[string]any{
		"before": e.Before,
		"after":  e.After,
	})
	if err != nil {
		return fmt.Errorf("audit: marshal metadata: %w", err)
	}
	// 即使 Before / After 都 nil，json.Marshal 也会产出 `{"before":null,"after":null}`,
	// 不会触发 JSONB NOT NULL 兜底，但仍保留一行防御性逻辑。
	if len(meta) == 0 {
		meta = []byte("{}")
	}

	row := map[string]interface{}{
		"created_at":  time.Now().UTC(),
		"actor_type":  string(e.Actor.Type),
		"action":      e.Action,
		"target_type": e.Target.Type,
		"reason":      e.Reason,
		"metadata":    meta,
	}
	if e.BrandID != nil {
		row["brand_id"] = *e.BrandID
	}
	if e.Actor.ID > 0 {
		row["actor_id"] = e.Actor.ID
	}
	if e.Target.ID > 0 {
		row["target_id"] = e.Target.ID
	}
	return tx.Table("operation_logs").Create(row).Error
}
