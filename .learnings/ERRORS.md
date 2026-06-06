## 2026-06-06 payment_callback_logs JSONB NOT NULL violated

**症状**：`POST /api/v1/public/payment/callback` 返 500。日志：
```
ERROR: null value in column "headers" of relation "payment_callback_logs" violates not-null constraint (SQLSTATE 23502)
```

**根因**：`PaymentCallbackLogModel.Headers` / `Payload` 字段是 `[]byte`，agent 实现未赋值 → GORM 把 nil 切片发为 SQL NULL（**不会**触发 column DEFAULT），列约束是 `JSONB NOT NULL DEFAULT '{}'` → 直接 23502。

**修复**：`commercial_models.go` 给 `PaymentCallbackLogModel` 加 `BeforeCreate(*gorm.DB) error` hook，nil 时填 `[]byte("{}")`。

**通用化教训**：GORM `[]byte` 字段映射到 PG JSONB 列时，应用层未显式赋值的情况都需要兜底；不要指望 DB DEFAULT 救场。其他可能受影响的列见 `commercial_models.go` 中含 `gorm:"type:jsonb"` 的字段。

### Pending exposure：其他 JSONB NOT NULL 列尚未加 BeforeCreate

通过 `grep "NOT NULL.*'{}'" migrations/000003_course_booking_schema.up.sql` 列出，迁移里至少还有 5 列是 `JSONB NOT NULL DEFAULT '{}'::JSONB`：
- `payment_callback_logs.headers` / `payment_callback_logs.payload`（Batch 3 已修）
- `operation_logs.metadata`（`OperationLogModel.Metadata`，`commercial_models.go:159`）
- `migrations/000003_course_booking_schema.up.sql:260, 1071, 1153, 1171` 还有几张表（含 brand_settings.setting_value 等）

`OperationLogModel.Metadata` 当前所有调用点都显式 `json.Marshal(...)` 赋了非 nil 字节切片，所以暂未爆雷。但只要后续有人写 `tx.Create(&OperationLogModel{...})` 忘记给 Metadata，就会复发同样 23502。建议要么给每个含 `JSONB NOT NULL` 的 model 加 BeforeCreate hook，要么写一个共用 `defaultJSONBField(field *[]byte)` 工具并在 PR review 时强制要求。

`migrations/000003_course_booking_schema.up.sql:1171` 的 `brand_settings.setting_value` 目前还没有对应 GORM model，引入时直接带上 hook。