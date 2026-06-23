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

## 2026-06-17 Batch 12b — code-review 修复 + 观察

### 修：repeat_weeks 模式回填派生 end_date 破坏 XOR（code-review）
service 原把算好的 endDate 无条件写进 GenerateInput.EndDate → repo 见非空就写 end_date 列，weeks 模式行同时有 repeat_weeks + end_date，违反二选一。修：落库传原始 in.EndDate（weeks 模式空→NULL）；occurrence 生成仍用本地算好的 endDate。

### 观察（非缺陷，回归确认）
- >200 节守卫被 26 周上限遮蔽（182<200），>200 分支结构上不可独立触发，仅纵深防御。
- class-sessions list DTO 不含 recurring_schedule_id（恒 null）；GET :id 聚合证明 FK 回填生效，属 DTO 取舍。
- exclusionConstraint 空约束名退化仍是 12a 已记 FR（pgx 恒填 ConstraintName）。

## 2026-06-18 Batch 13a — code-review 修复 + 转 FR

### P1：软删表的 unique 索引漏 `WHERE deleted_at IS NULL` → 删后无法重建（code-review，migration 000010）
`brand_learner_profiles` 的 `idx_brand_learner_profiles_brand_identity`(brand_id, learner_identity_id) 是无条件唯一，缺 partial filter。软删学员后，用同手机号（→find-or-create 命中同 identity）在本品牌重建 → INSERT 撞**软删死行** → 23505 → 误报 `LEARNER_ALREADY_EXISTS`，永久无法重建（List/Get 又看不到该行，体验割裂）。同表 `..._brand_learner_no` 和 `locations` 的唯一索引都带 `WHERE deleted_at IS NULL` 正为此。修：migration 000010 DROP+CREATE partial + `TestLearner_RecreateAfterSoftDelete`。
**Pending exposure**：凡 `gorm.DeletedAt` 软删表，逐条核对每个 UNIQUE INDEX 是否 partial（deleted_at IS NULL）。000003 里其他软删表的 unique 若有同类漏写，删后重建同键也会炸——下次碰到软删表先 `\d <table>` 核对索引谓词。

### 转 FR（非阻断）
- repo `UpdateStatus` 源态加固：service 层已挡 `inactive→frozen`（只接受 active/frozen 入参），但 repo 层不校验源态，未来若有别的调用方直连 repo 可能把 inactive 翻 frozen。低优先（当前唯一调用方是 service）。
- 学员服务关系 `learner_staff_assignments`（顾问/销售/主教练/跟进人）13a 延后：表已建，喂 instructor「自己相关学员」data_scope，C 端/instructor 视图批次再做。
- 学员批量导入（blueprint §20.7）；微信 openid 登录时按手机号回填合成 identity 的真实 open_id。
- 验收 finding（已改文档对齐，非代码 bug）：`QUOTA_EXCEEDED` 实际 409（SubscriptionGuard 自 Batch 4 约定），13a 契约误写 403。新批写错误码表先核对复用码的真实 HTTP status。

## 2026-06-18 Batch 13b — 验收阻断 F1 + code-review 修复 + 转 FR

### F1（验收阻断）：聚合 list 端点漏填内嵌字段 → 列表显示错 + 列表驱动编辑炸
`ListProducts` 只填 issued_count，没填 `location_ids/course_ids`（仅 `GetProduct` 填）。后果：① specific scope 产品列表显示「0 门店/0 课程」；② 前端产品页把 list 行当 `initial` 传进编辑弹窗 → scope chip 不回填 → scope=specific 但 selectedIds 空 → 保存撞「至少选 1 个」校验，无法编辑。用户验收 session 定位+修（加 `loadScopeIDs` 批量回填，镜像 loadIssuedCounts 一次 IN 避 N+1），本端收编 + 补回归单测 `TestEntitlementProduct_ListIncludesScopeIDs`（原测试只覆盖 GetProduct scope，故漏）。
**Pending exposure**：凡「前端从 list 拿对象后复用去编辑」的域，list DTO 必须填齐 detail 同款内嵌字段（scope ids / 反范式名 / counts）。13c booking 列表若被复用编辑同理；起飞前 grep 前端 `setEditing(rowFromList)` 之类用法核对。纯 curl 烟测（按 detail 端点取）查不出——必须走「list→编辑」UI 链路或显式断言 list DTO 字段。

### code-review 修复（无 P0/P1，3 项 P2 当批修）
- 门店 scope 校验 active-only→存在性（同课程对称）：避免产品建好后门店停用、编辑产品被 `ENTITLEMENT_SCOPE_INVALID` 强拒。
- `ListEntitlementsByLearner` 加学员属本 brand 守卫→`LEARNER_NOT_FOUND`（与 Grant 一致，不返空列表泄漏）。
- `SettleStatus` 纯函数补表驱动单测（之前只间接经 repo 测覆盖）。

### 转 FR（非阻断）
- 发放生效日 `starts_at`：前端 date→local 午夜→UTC，非 UTC 时区可能差一天；与 12b per-brand TZ 一并做。
- 产品次数 zod `min(0)` → 0 只触顶层 apiError 无 inline field error（cosmetic）。
- 权益到期/额度不足提醒通知（blueprint §20.13，通知批次做）。

## 2026-06-22 Batch 13c — code-review P0 + 转 FR

### P0：domain struct 直接 response 返回但漏 json tag → HTTP 层 PascalCase（struct 单测测不到）
`domain/booking.Policy` struct 无 json tag，`GetPolicy`/`UpsertPolicy` 经 `response.Success`(Gin 默认 encoding/json) 返回 → 响应体是 PascalCase(`BookAheadMinMinutes`)，前端按 snake_case 读全 undefined，`/booking-policy` 页读/写/存全坏。**后端 DB/service 单测用 struct 直传，结构上测不到序列化键名**；prod build 也不查运行时 JSON 形状 → 只有 HTTP/e2e 才现形。修：10 字段补 snake_case tag（可空 `*int` **不加 omitempty**，保留显式 `null`=不限）+ `TestPolicyJSONSnakeCase`（断言 `json.Marshal(DefaultPolicy())` 含 snake_case key + `:null`）。
**Pending exposure**：所有「`response.Success(c, 某 domain struct)`」路径都有此风险。本批其余 DTO（Booking/Hold/UsableEntitlement）有 tag 故没事，但 Policy 漏了。规则：凡 domain struct 经 REST 返回，要么加 json tag + 一个 marshal 形状断言，要么验收时 curl 核对响应 key 大小写——别只信 struct 单测。

### 转 FR（非阻断）
- 23514 CHECK 违反裸 500：`bookingConflictError` 只识别 23505；容量/余额 CHECK 正常路径被行锁+预检(`nr<0`)挡住不触发，但边角（外部直改数据后并发）会落 500 而非 409。建议加 23514→`SESSION_FULL`/`ENTITLEMENT_NOT_USABLE` 兜底分流（镜像 23505）。
- 频次限额跨场次 TOCTOU：`frequencyCounts` 未锁 booking 行，不同场次=不同 session 锁，同学员并发下两单不同场次可双双过频次预检。软策略轻微突破、无数据损坏（容量/课时仍有行锁+CHECK 强保证）。严格化需对 profile 行加 advisory lock。
- cancel_deadline 对员工代取消同样生效；员工绕过截止时间留 FR。

## 2026-06-22 Batch 13d — 验收 + 13c 经验生效

### 13c P0（domain struct 漏 json tag）经验成功阻断复发
13d 新 `waitlist.Entry` struct 从一开始就加 snake_case json tag（含反范式字段），code-review 显式复核确认无 PascalCase 问题——13c 那个「struct 单测测不到的 HTTP 序列化 P0」未在 13d 复现。跨批 learning 闭环有效。规则延续：凡 domain struct 经 REST 返回，建 struct 时即加 json tag。

### list 派生计数 stale：变更关联实体的 mutation 要失效 list query（验收期修）
场次行「候补 (N)」徽标由 session list 的 waitlist_count 子查询得出，但 waitlist 的 join/skip/cancel 改了活跃候补数却只失效 `brand-waitlist` → 徽标 stale。修：join/skip/cancel 追加失效 `brand-class-sessions`/`brand-class-session`。规则：A 实体 list 带了 B 实体的派生计数，B 的 mutation 要失效 A 的 list query。

### 开发期：候补重复预检顺序（DB 约束兜底但顺序影响错误码友好度）
limit=1 时同学员重复候补，若 limit 检查先于 partial unique 触发 → 报 WAITLIST_FULL 而非 WAITLIST_DUPLICATE（test 抓到）。修：加显式「已在候补」预检先于 limit 检查；partial unique 留并发兜底。规则：多个 409 检查并存时，把更具体/友好的（DUPLICATE）排在更泛的（FULL）前。
