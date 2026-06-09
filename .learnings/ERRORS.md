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

## 2026-06-06 Batch 5 验收期 bug

### `json:",omitempty"` 把空数组从响应丢字段，前端 `.map()` 炸

**症状**：Owner 账号刚注册、还没分门店任职时，`GET /api/v1/brand/staff/:id` 返回里 **没有** `location_assignments` 字段（不是 `[]`，是字段不存在）。前端 staff 详情页 `staff.location_assignments.map(...)` → `TypeError: Cannot read properties of undefined`。RoleAssignments 同款隐患——任何 staff 创建后角色未关联的瞬态都会复发。

**根因**：domain `Staff` 结构体的两个数组字段打了 `json:"...,omitempty"`。Go 的 encoding/json 对 nil slice 和 长度为 0 的 slice 都视为"空"，omitempty 直接丢字段（不是写 `null`、不是写 `[]`）。Repository 层 `toStaffDomain` 又允许 nil slice 透传——双重失守。

**修复**（commit `1b37b3a`）：
1. 域结构体去掉两个 `,omitempty`：`RoleAssignments []RoleAssignment \`json:"role_assignments"\`` / `LocationAssignments []LocationAssignment \`json:"location_assignments"\``。
2. `toStaffDomain` 入口规整 nil → 空切片，确保 `len==0` 时仍序列化为 `[]`。

**通用化规则**：
- **API 合约：任何前端会 `.map()` / `.length` / `forEach` 的数组字段都不要 `omitempty`**。omitempty 只适用于"前端读了就当没传"的可选标量（如 description）。
- **Repository / mapper 层有义务 nil → 空切片规整**——不能依赖上游 service 记得加。Batch 6 起 Course / Learner 的 domain mapping 一律加这层。
- 全仓 grep 排查未爆雷条目：`grep -RnE '\[\][A-Za-z].*omitempty' internal/domain/` 找出所有 `[]T ... omitempty` 字段，逐个评估前端是否会迭代。

### Handler 绑定 `string` 但前端发 `string[]`，被 Gin 直接拒为 INVALID_REQUEST

**症状**：教练资料 PUT `/api/v1/brand/staff/:id/instructor` 在前端弹窗保存时一律返 `400 INVALID_REQUEST {"error":"invalid request body"}`，service 层断点根本没进入。前端发的是 `{"specialties":["瑜伽","普拉提"],"certificates":["RYT200"]}`（chip 输入控件）。

**根因**：handler `upsertInstructorBody` 把两个字段绑成 `Specialties string` / `Certificates string`，Gin `ShouldBindJSON` 走 encoding/json 反序列化，类型不匹配立即返 unmarshal 错误并被 handler 转成通用 `INVALID_REQUEST`，**错误信息不带字段名**——只有 access log 里 raw body 才看得出来 specialties 是 JSON array。DB 列 `VARCHAR(1000)` 设计是逗号分隔 CSV，但 Handler 既没有 alias 也没有显式 unmarshal，前端无法自察。

**修复**（commit `97a33bb`）：
1. Handler 绑成 `[]string`，内部 `joinCSV()`（trim + 跳空）合成逗号字符串再交给 service / DB，保持 domain / DB schema 不变。
2. Response 用 embedding 结构 `instructorProfileResponse` 把 domain.Profile 的 CSV `splitCSV()` 还原成 `[]string`，对称前端写入形状。

**通用化规则**：
- **DB 列即便是单 `VARCHAR`，handler 也接受 `[]string`**，内部 join/split 桥接。理由：前端控件（chip / tag input）天然产出数组；如果让前端 join，每个调用点都要复制 trim/dedup/skip-empty 逻辑，必然漂移。
- Handler 加 helper `joinCSV` / `splitCSV` 统一 trim + 跳空 + 可选 dedup，杜绝"前导/尾随空逗号"脏数据进库。
- 通用 INVALID_REQUEST 调试线索藏在 access log 的 raw body；建议下一批给 `response.Error` 在 bind error 分支带上 unmarshal 错误的 field path（`json.UnmarshalTypeError.Field`），调试链路缩短一个量级。