## 2026-06-06 Batch 3 衍生 follow-up

- **CI 加 `go generate ./... && git diff --exit-code` 校验**：Batch 3 出现 `wire_gen.go` 被手改 + `// TODO wire regen` 注释。需要门禁在 PR 阶段阻断这种漂移，避免合并后 service 构造器与三个 wire_gen 不一致。
- **拆 `commercial.PublicHandler`**：当前 brand server 复用 `adminHandler.NewHandler(nil, commercialSvc, nil, nil)` 仅为挂 `RegisterPublicRoutes`。建议把 `signup/sms-code`、`signup/orders`、`payment/native`、`payment/callback` 等公开接口拆到独立 `interfaces/public/handler.go`，brand / admin / app 三端都能干净复用。
- **抽 `pkg/money` 包**：`parseAmountToFen` / `fenToAmountString` 当前散落在 repository 里，自实现截断而非四舍五入，未来对接多币种或第三方对账时会踩坑。建议引入 `shopspring/decimal` 或定义 `Money int64`+`Format/Parse`。
- **统一 mock 模式判定**：在 `config.Config` 上加 `IsMockSMS()` / `IsMockPayment()` 方法，把"AllowMock / Provider / Env / AppID"四个判断收敛到一处，避免 staging 环境某些字段未配置时行为不可预期。
- **真实微信 v3 验签 + AES-256-GCM 解密**：`wechat_adapter.go` 的生产路径目前返回 `errors.New("not implemented")`。下一阶段需要：拼接 `timestamp\n + nonce\n + body\n` 待签字符串，按 `Wechatpay-Serial` 选平台证书 RSA-SHA256 验签，AES-256-GCM 解密 `resource.ciphertext`。

## 2026-06-06 Batch 4 code-review 转移项

下列是 Batch 4 post-impl code-review 中明确为"非阻塞、留作未来批次"的发现：

- **BRAND_NOT_ACTIVE 文案 + profile 守卫**：目前对 `pending` / `disabled` / `frozen` 统一返"品牌未激活"，对被运营冻结的老品牌混淆体验；同时 PATCH /profile 未加同款守卫，被冻结的品牌仍能改资料。需要：(a) 拆 BRAND_PENDING / BRAND_DISABLED / BRAND_FROZEN 三个错误码；(b) 给 brand profile / 后续所有 brand-side 写接口统一加 active 守卫中间件。
- **providePublicHandler nil-injection 风险**：`cmd/api-brand/wire_gen.go` 通过 `adminHandler.NewHandler(nil, commercialSvc, nil, nil)` 复用 admin handler 只为挂 public routes；未来若给 admin handler 加方法、不慎挂到 public group 上 → 生产 nil-panic 且 CI 无屏障。彻底解法是新建 `interfaces/public/handler.go`（见 Batch 3 已有的 PublicHandler 提案），把所有公开接口迁过去。
- **JSONB BeforeCreate hook 散落**：当前 PaymentTransactionModel / SaaSPlanOrderModel / PaymentCallbackLogModel / OperationLogModel / BrandOnboardingStepModel 都各自实现近乎一字不差的 `if len(x)==0 { x=[]byte("{}") }` hook。下一个新 model 100% 会忘记复制 → 复发 23502。建议抽 `type JSONB []byte` 实现 `Value()`/`Scan()`/`BeforeCreate`，或 GORM plugin 扫 tag 自动补默认。
- **SubscriptionGuard 抽出**：Location quota check 当前内联在 `location_repository.Create` 的事务里。Staff / Learner 即将到来的同样模式（SELECT FOR UPDATE active subscription + COUNT + 比 max）应抽到 `internal/application/commercial/subscription_guard.go` 复用，否则三处会各自轻微漂移。
- **OperationLog 两套 helper 合并**：`createOperationLog` (commercial_repository.go, actor=platform_admin) 和 `writeLocationOperationLog` (location_repository.go, actor=brand_user) 结构同源、actor type 不同。下一个 batch 加 staff/learner/category lifecycle 时建议先抽 `internal/audit` 包：`audit.Write(tx, audit.Event{...})`，统一 metadata 序列化和 actor type 枚举。
- **错误码 → HTTP 状态 表格化**：`apperr.NewAppError(apperr.ErrXxx, "msg", 4xx)` 在本批已出现 14+ 次。建议在 `pkg/errors` 加一个 `codeMeta` 映射 + `Raise(code, opts...)` helper，避免错误码改 HTTP 状态时漏改某个 raise 点。
- **onboarding 流程的 7 表 COUNT 一次性聚合**：目前 7 个串行 roundtrip。下一批 staff/course CRUD 落地后查询频次会上升，建议改成单 SQL 的 UNION ALL 或带 Redis 短 TTL 缓存（key=brand_id），降低 GET /onboarding/status 的 P95。
- **Location Update 的 SELECT-UPDATE-SELECT 三步**：可用 GORM `clause.Returning{}` 在 Postgres 上一次拿到更新后的行。同样适用于 brand profile PATCH。属于后续 batch 顺手清债。
- **Location UpdateStatus 幂等快路径**：若 status 没变化，当前仍开事务 + SELECT。可以用 `UPDATE ... WHERE status <> ?` + RowsAffected 判断，跳过整个事务。也属于性能清债。
- **Step enum 表格化**：`AllSteps` / `IsValidStepKey` / `IsSkippable` + onboarding service 的大 switch 都从同一份 step 列表派生。建议改成 `var stepCatalog = []StepDef{{Key, Skippable, Order, CountSource}}` + `init` 建 by-key 索引，下一次加 step 时只改一处。
- **brand_profile 完成判定加 logo_url**：当前只看 description + industry_type，但 c 端品牌头会渲染 logo。建议在产品确认后加 logo_url 必填校验（或 fallback 占位图策略）再翻 step 完成。

## 2026-06-06 Batch 4 → Batch 5 起飞前必做

- ~~**migration 自动化 / schema drift 防御**~~：✅ Batch 4.5 已落地（commit `13bdcd2..696d02d`）。auto-apply on boot + Makefile DSN fallback 都已上线。CI 真 PG 跑测试仍未做，但 boot-time 已足够覆盖本地开发主线场景。
## 2026-06-06 Batch 5 code-review 转移项

7 项 P1/P2 在 Batch 5 当批合入（commits `9cd2807` backend / `36bd170` frontend）；以下转下批清债。

- **B1 service.Create 原子性**：staff_repository.Create / ReplaceRoleAssignments / ReplaceLocationAssignments 当前是 3 个独立事务，第 2/3 步失败留下孤儿 staff 占 seat quota（E36 配额回滚不一致）。需要在 staffRepository 加 `CreateWithAssignments(ctx, brandID, actorID, in, roleAssignments, locationAssignments)` 单事务方法。
- **B6 OwnerRoleAllocator 接口加 ctx**：`role_allocator.AssignDefaultOwnerRolesTx` 现在用 `context.Background()`，丢失 HTTP 请求的 cancellation / deadline / trace。接口需要破坏性改成 `AssignDefaultOwnerRolesTx(ctx context.Context, tx any, brandID, brandUserID int64)`，commercial.Service 注册流程 + admin backfill 各传一份 ctx。
- **B7 providePublicHandler 真重构**：`cmd/api-brand/wire_gen.go:81` 的 `admin.NewHandler(nil, commercialSvc, nil, nil, nil)` 现在 4 个 nil（Batch 5 加了 SystemHandler 后 +1）。Batch 4 FR 已列过，本批又加深一层。彻底方案：抽 `internal/interfaces/public/handler.go` 把 RegisterSaaSPlanRoutes / RegisterPublicRoutes 等真公开接口搬过去，admin Handler 不再被 brand 进程复用。
- **B12 主门店单选 invariant 客户端化**：`staff-create-dialog.tsx` 的 `primaries <= 1` schema 没有强制 "assignments 非空时必须正好 1 primary"；服务端会以 LOCATION_ASSIGNMENT_INVALID 兜底但 UX 不直观。RoleAssignmentEditor 同款问题（参考 backend correctness finder #3）。
- **B13 scope-aware location 在 create dialog 缺失**：StaffCreateDialog 现在只问 role_codes 多选 + location_assignments 多行，没在客户端先判断"选了 location-scope 角色但没填 location_assignment"。RoleAssignmentEditor 有，create dialog 没有。
- **B14 ResourceStatusToggle 泛型抽出**：StaffStatusToggle 是 LocationStatusToggle 的几乎逐行复制；Learner / Course 一定会再来一份。建议抽 `<ResourceStatusToggle<T extends {id, status, name}> mutation={...} ownerLocked={...} />` 把 mutation hook + error code 映射当 props 传进去。
- **B15 instructor_repository action 区分 created/updated**：`instructor_profile_upserted` 硬编码，BI / 合规报表 SQL filter 写起来啰嗦。before==nil 时记 `instructor_profile_created`，否则 `_updated`。
- **B16 staff_seats COUNT 是否过滤 status='active'**：当前 SubscriptionGuard 数 brand_users 不过滤 status，inactive 也占席位（与 Location 同款"防 disable→腾位 hack"）。前端"停用员工"按钮可能让用户误以为腾位置；需要产品决定意图后再调整 COUNT 或加文案。
- **B17 audit.Write BrandID==&0 兜底**：audit pkg 接受 `*int64` BrandID，零值指针会被写入 brand_id=0 行；建议加 `if e.BrandID != nil && *e.BrandID > 0` 守卫。
- **B18 staff_repository.List HasInstructor 分支不对称**：true 分支用 `status='active'`，false 分支用 NOT EXISTS 不限 status。inactive instructor 在两边都漏。统一过滤策略。
- **B19 AssignDefaultOwnerRoles ON CONFLICT 部分索引盲点**：唯一索引 WHERE status='active' AND location_id IS NULL；如果存在历史 status='inactive' 的 brand_owner 关联，ON CONFLICT 不命中会重复插入。改 `INSERT ... WHERE NOT EXISTS` 或先 UPDATE 拉起再 INSERT。
- **B-delete-condition 扩展**：详情页删除按钮 disabled 条件目前只看 `is_owner`；后端将来扩 OWNER_PROTECTED 触发条件（如"最后一个 admin"）时 UI 不会预禁用，按钮可点 → 失败 toast。建议让后端先返回一个 `can_delete: false, reason: 'last_admin'` 字段，前端按它渲染 tooltip。

## 2026-06-11 Batch 6 → Batch 7 延后项

- **GET /roles/:code 单条角色详情**：Batch 6 只做了 GET /me/permissions（当前用户有效权限+data_scope）。品牌后台"角色管理"页需要按 code 查单个角色的权限明细，留 Batch 7 自定义角色 CRUD 一并做。
- **GET /permissions 全量权限列表**：自定义角色编辑器需要一份"所有可分配 permission code + 中文名 + 分组（域）"的元数据列表给前端勾选。SoT 是 permissions 表，加一个只读 endpoint 即可。
- **品牌自定义角色 / 调整权限 CRUD**：Batch 5 只 seed 了 8 个预置 role_templates → brand_roles；品牌后台自建角色、改角色权限、删角色的完整 CRUD 仍未做（Batch 5/6 两次列入，是 Batch 7 主线）。
- **T10 完整回归未自动跑**：Batch 6 的 35 个测试场景（H1-H6 happy path + E1-E35 edge）只做了人工抽样验收，未生成端到端 Playwright/集成测试。建议 Batch 7 起飞前补关键路径（权限拒绝 403、data_scope 越权 404、owner fast-path）的自动化覆盖。

## 2026-06-12 Batch 7 code-review 转移项

Batch 7（自定义角色 CRUD）post-impl code-review 中，2 项已当批修掉（commit `0950200`：role 响应补 Permission.Description；DeleteRole 计数改 status-agnostic + 删死代码）。以下转后续清债：

- **共享 `isUniqueViolation` 真有 bug（8 个 repo 受影响，P1）**：`user_repository.go:302-318` 的 `containsUniqueConstraint` 与 `isPGUniqueViolation` 是逐字节相同的复制（"PG" 变体名不副实，根本没调 `errors.As`/pgconn），且靠英文前缀 `msg[:27] == "ERROR: duplicate key value"` 匹配（locale/格式脆弱），`msg[len(msg)-10:]` 对短于 10 字节的错误串会 panic（仅 `len(msg)>0` 守卫）。Batch 7 在 `role_repository.go` 新增了正确的 `isUniqueViolationPG`（pgconn code 23505）绕开它，但其余 instructor/staff/user/brand/location/commercial/brand_extension 8 处仍走旧的脆弱实现。建议：抽 `pkg/pgerr.IsUniqueViolation(err)` 用 `errors.As(*pgconn.PgError)` + `SQLSTATE==23505`，全量替换并删掉旧 helper。
- **`GetRole` 双查（效率）**：`service.go:GetRole` 先 `GetBrandRoleByCode` 再 `ListBrandRoles(brandID)` 全量拉所有角色 + JOIN 全部权限，只为线性扫出单条的 permissions。`role_repository.go` 已有 `getBrandRoleByIDWithPermissions` 做的正是单角色+权限，但未导出。建议导出/加接口方法，`GetRole` 直接按 ID 取，省掉 O(N) 扫描。
- **角色缓存批量失效是逐 key DEL（效率）**：`invalidateRoleHolders` 拿到全部 holder id 后逐个 `checker.Invalidate` → 单 key Redis DEL 往返。改 widely-assigned 角色（如 200 员工）会在请求路径上打 200 次串行 DEL。建议 `checker` 暴露 `InvalidateMany([]int64)`，一次多 key DEL / pipeline。
- **`resolvePermissionIDs` 拒绝 inactive code（潜在）**：全量替换权限时若回传的 code 里含已 `status=inactive` 的权限，整批被拒（连改名也存不了）。当前无任何流程会把 14+1 个 permission 置 inactive，故不可达；若将来引入权限下线流程需重审：编辑既有角色时对"已存在于该角色、但现已 inactive"的 code 应放行而非拒绝。
- **`/roles/:id` 路由参数名语义误导（可维护性）**：role 路由注册为 `:id` 但 handler 按字符串 code 解析（`c.Param("id")`），而同 handler 的 `/staff/:id` 是数字 ID（`ParseInt`）。同名参数两种含义，易被后人 copy 错。建议 role 路由段改名 `:code` 自文档化（注意 Gin 同前缀冲突规则，`/roles/*` 与 `/staff/*` 前缀不同，可安全改名）。
- **`brand_owner`/`is_system` 保护检查散落（altitude）**：`resolveRoleAssignments`、`ReplaceRoleAssignments`、`requireMutableCustomRole` 各自硬编码 `code == "brand_owner"` 字面量（3+ 处）+ 重复 `IsSystem` 判断。建议在 `role.BrandRole` 上收敛为 `IsProtected()` / `IsMutable()` 谓词，单点维护这条安全不变量（注释已自承"security-critical"）。

## 2026-06-16 Batch 11 → Batch 12 候选

- **RecurringSchedule 循环批量排课**：recurring_schedules + recurring_schedule_weekdays 已建表无 CRUD；按 weekly pattern 批量生成 class_sessions，逐场次跑教练/资源冲突。
- **Location Resource 管理 + 资源时间冲突**：location_resources CRUD；class_sessions.location_resource_id 本批恒 null，绑定后启用 `class_sessions_resource_no_overlap` EXCLUDE（已在 DB）→ SESSION_RESOURCE_CONFLICT。
- **own_sessions 数据权限**：教练只看自己授课场次（DataScope 目前只实现 all_brand / assigned_locations；instructor 角色 session.view 现等同 assigned_locations）。
- **场次改期 reschedule**：本批只 create + cancel，改时间靠取消重排。
- **CourseCategory DELETE 接口**：当前只能 PATCH status=inactive，无硬删（e2e teardown 只能停用，分类行累积）。
- Booking/Waitlist/Attendance/EntitlementHold（学员预约批次）落地后，门店/课程删除 guard 再纳入对应引用。

## 2026-06-16 Batch 12a 转移项

- **Batch 12b：RecurringSchedule 循环排课**（recurring_schedules + recurring_schedule_weekdays 表已建）。grill 已定：单 tx + 逐 occurrence SAVEPOINT 部分成功（冲突跳过返清单）｜0 成功 abort 返 409+skipped 不落空壳｜非级联 cancel（status→cancelled 不动已生成场次）+ 复用 session.* 权限｜生成时区 Asia/Shanghai｜门店删除 guard 纳入 active recurring_schedules。
- exclusionConstraint 空约束名退化时的资源/教练冲突歧义（见 ERRORS）。
- 资源 Delete guard TOCTOU（见 ERRORS）。
- 排课/编辑场次时「所选资源已停用」前端提示（blueprint §20.5）。
- 资源占用日历视图（blueprint §20.5「查看资源占用日历」）。
