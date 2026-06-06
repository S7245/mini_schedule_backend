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