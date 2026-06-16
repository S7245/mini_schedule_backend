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

## 2026-06-11 Batch 6 service_test 因新增 PermissionChecker 依赖踩坑

### 存量 service 单测在 service 构造器加 checker 形参后编译 / 行为双重断裂

**症状**：staff / onboarding service 的 `NewService` 签名末尾加 `checker PermissionChecker` 后，存量测试要么编译失败（`not enough arguments in call to NewService`），要么编译过但 happy-path 用例突然返 404 / PERMISSION_DENIED——明明没改业务逻辑。

**根因（两层）**：
1. **构造器形参变更未同步存量调用点**：所有 `NewService(repo, roleRepo, instrRepo)` 调用少一个参数直接编译炸。
2. **fake checker 的零值 `DataScope` 不是"放行"而是"拒绝所有"**：`fakePermissionChecker.Resolve` 默认返 `domainrbac.DataScope{}`（`Kind == ""`）。而 `scopeFilterIDs` 对 `Kind==""`（DataScopeNone / 未知）一律映射成 `[]int64{}`（reject-all，fail-closed），于是详情/列表 happy-path 全被 scope 守卫挡成 404 / 空列表。开发者第一反应是"权限逻辑写错了"，实则是 fake 的默认 scope 没设。

**修复**：
1. 加一个 `newSvc(...)` helper 用 **nil checker**（走 bypass）保留所有不关心权限的存量用例不动；另加 `newSvcWithChecker(...)` 专门给 Batch 6 新增的 `Test*_Requires*` 权限闸门用例。两条构造路径并存，存量测试零改动。
2. 凡是要跑通 happy-path 又注入了 fake checker 的用例，**必须显式设 `resolveScope: domainrbac.DataScope{Kind: DataScopeAllBrand}`**，否则零值 scope = reject-all。
3. onboarding test 顶部补 `domainrbac "github.com/zkw/mini-schedule/backend/internal/domain/rbac"` import（fake checker 的 `Resolve` 签名引用了 domain 类型），漏补就是 missing-import 编译错。

**通用化教训**：
- **给构造器尾部加依赖，优先用"nil = bypass"语义 + 一个保留旧签名的 helper**，让存量测试零改动，只给新行为写新用例。比"全量改调用点 + 给每个 fake 补默认实现"成本低、回归面小。
- **fake 的零值默认必须对齐生产 fail-closed / fail-open 取向**：本批 scope 是 fail-closed（零值 = 拒绝），所以 fake 默认零值 = 拒绝是一致的——但它会让"忘记设 scope"的 happy-path 用例静默变 404，排查时先查 fake 的 scope 设没设，再怀疑业务逻辑。Course / Learner 接 checker 时同款陷阱。

### Pending exposure：DataScope.LocationIDs 仍带 `omitempty`，前端迭代会复发 Batch 5 同款 TypeError

`internal/domain/rbac/permission.go:98` 的 `LocationIDs []int64 \`json:"location_ids,omitempty"\`` 以及 `internal/interfaces/brand/me_handler.go:36` 的 `meDataScopeBlock.LocationIDs` 同样打了 `omitempty`。这正是 Batch 5 验收期 staff `location_assignments` 踩过的"前端会 `.map()` 的数组字段不要 omitempty"坑（见本文件 Batch 5 条目）。

当前 me_handler 用了"仅当 `Kind==AssignedLocations` 才填 LocationIDs"的写法**部分**遮蔽了风险：all_brand / none 时前端本就不该读 location_ids。但只要出现 `Kind==AssignedLocations` 且 `LocationIDs` 为空切片（如 scope 配了 assigned_locations 但还没分任何门店的瞬态），`omitempty` 仍会把字段丢掉，前端 `data_scope.location_ids.map(...)` 复现 `Cannot read properties of undefined`。

**建议**：me_handler 的 `meDataScopeBlock.LocationIDs` 去掉 `omitempty`，并在 handler 出口 nil → `[]int64{}` 规整（对齐 Batch 5 给 staff 数组字段定的规则）。domain 层 `DataScope.LocationIDs` 的 omitempty 是否保留可商榷——它同时进 Redis 缓存的 JSON（`cachedResolve.Scope`），缓存形态空切片 vs 缺字段对 `MergeScopes` 无影响（反序列化都得 nil slice），所以 domain 层保留 omitempty 无害；**但凡是直接出给前端的 response 结构体一律去 omitempty**。全仓复查：`grep -RnE '\[\][A-Za-z].*omitempty' internal/interfaces/` 找出所有出 handler 的数组 omitempty 字段。

### Pending exposure：RBAC L1 缓存按 brand_user 单维度 key，跨 brand 复用同一 brand_user_id 会串权限

`application/rbac/checker.go` 的 L1 缓存 key 是 `cacheKeyForUser(brandUserID)` = `rbac:perms:<brandUserID>`，**只含 brand_user_id 不含 brand_id**。当前数据模型下 brand_user_id 全局唯一、且天然属于单一 brand，所以不会串。但这是一个隐含不变量——**一旦未来 brand_user 支持跨 brand（同一自然人在多 brand 任职、复用同一 brand_user 记录），单维度 key 会让 A brand 的权限集泄漏到 B brand 的请求**。

ctx-cache 那一层 key 是 `requestKey(brandID, brandUserID)`（含 brand_id，正确），唯独 Redis L1 漏了 brand_id。**建议**：即便当前不变量成立，也把 L1 key 改成 `rbac:perms:<brandID>:<brandUserID>` 提前对齐 ctx-cache 维度，消除这个"靠数据模型巧合保证正确"的脆弱依赖；改 key 只需配合 TTL 自然过期，无需手动清缓存。
## 2026-06-12 Batch 7

- **手机号重复新增返回 500 而非业务错误（线上活 bug，已修）**：`POST /brand/staff` 手机号唯一冲突时本应返 `STAFF_PHONE_DUPLICATED`(409)，实返 `INTERNAL_SERVER_ERROR`。根因：共享 `isUniqueViolation`（user_repository.go）用英文前缀字符串匹配，pgx 错误串无该前缀 → 漏判 → 唯一冲突分支没进。影响 ~11 处调用点（staff/user/brand/location/commercial/instructor）。修：改 `errors.As(*pgconn.PgError)` + code 23505（commit `72f8583`），string 匹配仅作兜底。**Pending exposure**：所有依赖该 helper 把唯一冲突转业务错误的路径之前都可能在生产返 500，值得回归各注册/创建入口。
- **旧二进制掩盖新逻辑（验收期踩坑，非 bug）**：B1 增量逻辑改完 `go build` 通过，但 :8081 上跑的是改码前启动的 `go run` 进程，验收"看似失效"返旧行为。重启后端即正常。教训：本地验收前务必重建/重启后端；CI/部署需确保用最新源码构建。

## 2026-06-16 Batch 11

### 前端假设的后端端点从未实现 → 排课全链路 UI 阻断（验收期 e2e 抓到，commit dfda127）
`web/packages/api/src/instructor.ts` 早注明 `ASSUMPTION (backend must match): GET /api/v1/brand/instructors?schedulable=true`，但后端从未注册该路由 → 排课弹窗「可排课教练」下拉调用返 404 → 下拉恒空 → H5/E7/onboarding 全阻断。数据正常（张三 instructor_profile id=1 active+schedulable）。修：instructor.Repository 加 `ListSchedulable` + staff.Service `ListSchedulableInstructors`（门 instructor.view）+ `GET /instructors` handler，返回 `{items:[{id=instructor_profile_id,display_name,status,is_schedulable}]}` 与 `POST /class-sessions` 的 instructor_profile_id 入参对齐。
**Pending exposure**：凡前端 api client 里写了 `ASSUMPTION (backend must match)` 的端点，契约 API 接口表必须显式列入并后端落地；grep 全仓 `ASSUMPTION` 找漏网。
**教训**：纯 API curl 烟测**硬编码 instructor_profile_id 绕过了下拉**所以没暴露——走 UI 选择器的 e2e 才抓到。涉及「前端下拉/选择器拉某列表」的功能，烟测必须走 UI 或显式调那个 list 端点，不能只测最终 POST。

### CourseTemplate 更新传空 location_ids 误回填全部门店（code-review）
`resolveLocationIDs` 把空 ids 当「默认全选 active 门店」——create 时正确（契约默认），但 update 时用户取消勾选全部门店保存会被静默回填成全部。修：加 `defaultAllWhenEmpty bool`，仅 create 传 true；update 传 false，尊重显式清空。

## 2026-06-16 Batch 11 验收期续 — /staff/:id 详情漏内嵌 instructor_profile

### 已建教练档案的员工详情页恒显示「未启用」
`GET /staff/:id`（repo `GetWithAssignments`）只回 `has_instructor` 布尔，从不内嵌 `instructor_profile` 对象——但前端 `staff.Staff` 类型有 `instructor_profile?`，`InstructorProfileSection` 算 `hasProfile = has_instructor && profile`，profile 永远 undefined → 永远「未启用」，连「已启用/已停用」徽章和档案字段都不渲染。`packages/api/src/instructor.ts` 的 invalidate 注释甚至写明「Detail returns embedded instructor_profile」——前端早按内嵌设计且已 invalidate staff detail，是后端从 Batch 5 起就漏了这块（直到 Batch 11 排课要用教练才被翻出来）。
修（后端）：staff 域加 `InstructorProfileView`（specialties/certificates 数组化，与 `/staff/:id/instructor` 同形）+ `Staff.InstructorProfile` 字段；repo `GetWithAssignments` 调 `fetchInstructorProfileView` 内嵌（无则 nil→null）。`GetByID`（轻量存在性检查）保持不内嵌。
**坑中坑**：repo 有两个 getter——`GetByID`（轻量，role/loc/instructor 全空）只供存在性校验，详情走 `GetWithAssignments`。给详情补字段/写详情测试都要认准 `GetWithAssignments`，测 `GetByID` 会假阴性。
**同源教训**：又是「前端假设后端返某结构，后端没实现」（同本批 GET /instructors）。规则：前端 type 里 staff/课程等聚合根带的内嵌子对象（`instructor_profile?` 之类），后端聚合根 DTO 必须真填；起飞前对前端聚合 type 逐字段核后端是否返。

## 2026-06-16 Batch 12a — code-review 修复 + 转 FR

### 修：location_resource_id 误传 0 → 404（code-review）
handler body `*int64` 无 >0 校验，client 传 `{"location_resource_id":0}` → 非 nil 指针 → repo `First WHERE id=0`→RESOURCE_NOT_FOUND 404（应是「不绑定」）。修：classsession.Service.Create 把 `*id<=0` 归一为 nil。前端虽传 null，后端兜底。

### 修：资源 Update 的 Updates/re-read Where 只按 id（code-review）
`tx.Model(&LocationResourceModel{}).Where("id = ?", id).Updates(...)` 仅靠前置 brand 域 before 读保证隔离，单点依赖。修：Updates + re-read 的 Where 都加 `AND brand_id = ?`，防御性，与 Delete 路径一致。

### 转 FR（非本批阻断）
- exclusionConstraint 字符串退化路径（errors.As 失败、仅 `strings.Contains("SQLSTATE 23P01")`）返空约束名 → sessionConflictError 默认 INSTRUCTOR，资源冲突被误标。pgx 实际恒填 ConstraintName，纯理论；但退化时不可区分。
- 资源 Delete guard TOCTOU：COUNT 0→软删 与 并发 session-create 绑该资源 之间有窗口，可留下「active 场次引用已软删资源」。与现有 LOCATION_IN_USE/COURSE_IN_USE 同类 race（本仓约定靠 DB 约束兜并发，check-then-act guard 固有此窗口）。
- 停用资源不挡已排未来场次（blueprint §20.5 设计：不自动取消，但「编辑场次时提示资源已停用」——该提示未实现）。
