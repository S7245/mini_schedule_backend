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

## 2026-06-06 migration 000004 本地未应用

**症状**：Batch 4 实现完成、单元测试全过、但本地起 `api-brand` 跑 `PATCH /brand/profile` 写 `description` 列时报 `column "description" of relation "brands" does not exist`。

**根因**：`backend/Makefile` 的 `migrate-up` target 把 DSN 硬编码成 `postgres://postgres:postgres@127.0.0.1:5432/...`，但开发机的本地 PG 实例 default superuser 是 `liushan`（macOS Homebrew `brew install postgresql` 默认行为），没有 `postgres` 角色。`make migrate-up` 静默失败或报 `FATAL: role "postgres" does not exist`，开发者以为 migration 已经在容器里跑过了，实则 000004 从未应用。

**修复（一次性）**：
```bash
psql -d mini_schedule -c 'ALTER TABLE brands ADD COLUMN IF NOT EXISTS description VARCHAR(2000);'
```
或临时改 Makefile DSN 走 `$USER` / 本地 trust 配置后再 `make migrate-up`。

**通用化教训**：
1. 任何 "feature 依赖新 migration" 的 batch 必须验证 migration 在本地 / 测试环境真跑过——单测用 sqlmock 不能证明 schema 已升。
2. Makefile 的硬编码 DSN 是开发体验黑洞，应改为读 `DATABASE_URL` 环境变量（生产 Railway 已经走这套）；或加 `make migrate-up-local` 用 `${PG_USER:-$USER}` 兜底。
3. 启动 `api-*` 时建议加可选的 "schema drift check"：把 `migrations/*.up.sql` 的预期表/列与 `information_schema` 实际对比，drift 时打 warning（生产关闭）。
4. 下一批起飞前先把 migration 自动化（boot 时 `migrate.Up()`）或在 CI 加 schema 校验，否则会重复踩这个坑。