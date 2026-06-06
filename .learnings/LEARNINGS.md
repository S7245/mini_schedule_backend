## 2026-05-27

- 在 Codex sandbox 内运行 `go test ./...` 可能因为写入 `/Users/liushan/Library/Caches/go-build` 被拒绝；复跑时需要提升权限。
- 后端商业化迁移可用临时 PostgreSQL 验证，sandbox 会阻止 `initdb` 创建共享内存；启动本地临时 PG 需要提升权限。
- `self-improving-agent` hooks 已和项目级 `.learnings` 形成约定关联：当前目录或父目录存在 `.learnings` 时，Bash 失败写入 `ERRORS.md`，会话结束写入 `LEARNINGS.md`。

## 2026-05-28 skill routing memory

- Before backend implementation, use `backend/SKILLS.md` as the local skill router.
- Select only skills that are available in the current session; if a listed skill is unavailable, use the closest available local Go/database/architecture skill.
- For the current Go backend stack, default shortlist is `go-style-core`, `go-packages`, `go-error-handling`, `go-context`, `go-testing`, `database-schema-designer`, `clean-architecture`, and `domain-driven-design`.

## 2026-06-06 Batch 3 微信支付回调

- **JSONB NOT NULL 列 + GORM `[]byte` 字段陷阱**：`payment_callback_logs.headers` 和 `payload` 是 `JSONB NOT NULL DEFAULT '{}'`。GORM 模型用 `[]byte` 字段，未赋值时是 `nil` 切片，pgx 驱动会发 NULL（不是 SQL 层的 DEFAULT），直接撞 NOT NULL。修复方式：模型加 `BeforeCreate` hook 默认 `[]byte("{}")`。所有 JSONB NOT NULL 列建议都这样兜底。
- **CallbackLog 事务外补写策略**：仓储事务正常 commit 时（含 ignored / failed 业务分支），CallbackLog 在事务内写；只有事务真 rollback（err != nil）才在 service 层调用独立的 `WritePaymentCallbackLog` 补一条 failed 日志。验签 / timestamp 失败同理在 service 层补写——这些场景根本没进事务。
- **支付回调幂等判定**：只看 `order.status == 'paid'`，不强校验 `third_party_trade_no` 一致。理由：补单 / 异常订单可能换 tx_id 重发，强校验易误判。审计靠 `PaymentCallbackLog` 多条 processed 留痕。
- **回调 HTTP 响应码策略**：仅验签 / timestamp 失败返 401；订单不存在 / 金额错 / 非 SUCCESS state 一律 200，避免微信无意义重试（微信对非 200 会以指数退避反复重试，会放大事故）。仓储事务真出错才 500。
- **Mock 微信支付 adapter 约定**：`Wechatpay-Signature: mock_signature` 视为通过；body 直传明文 JSON（不做 AES-GCM 解密）；timestamp 仍校验 ±5min 窗口。实现挂在 `infrastructure/payment/wechat_adapter.go`，真实模式（cfg.AllowMock=false 且 AppID 非空）直接返 "not implemented" 错误，强制要求商户证书路径。

## 2026-06-06 Batch 3 patterns deep dive

- **Wire 漂移：手改 wire_gen.go 是技术债信号**。Batch 3 的 `cmd/api-brand/wire_gen.go` 和 `cmd/api-admin/wire_gen.go` 都被手动塞了 `wechatAdapter := payment.NewWeChatPaymentAdapter(cfg)` 并打了 `// TODO wire regen` 注释。`commercial.NewService` 签名增加了 `client` 和 `wechatAdapter` 两个参数，但没跑 `go generate ./...`。后续每次改 service 构造器都要同步三份手改文件，错误率高。每个 PR 应在 CI 增加 `go generate ./... && git diff --exit-code` 校验。
- **Public handler 复用 Admin Handler 是代码气味**。`cmd/api-brand/wire_gen.go` 出现 `adminHandler.NewHandler(nil, commercialSvc, nil, nil)`——四个依赖里三个 nil，只为复用 `RegisterPublicRoutes`。这种 nil 注入意味着 `admin.Handler` 应该拆出独立的 `PublicHandler`（或 `commercial.PublicHandler`），后续如果在 admin Handler 上加新依赖，brand 这边的 nil 列表会越来越长、漏更新就 panic。
- **Mock 开关判定不统一**。`cfg.SMS.AllowMock` 的实际判定是 `AllowMock || Provider=="mock" || Env!="production"`（见 `internal/application/commercial/service.go:304-307`），而 `cfg.Payment.WeChat.AllowMock` 是 `AllowMock || AppID==""`（见 `internal/infrastructure/payment/wechat_adapter.go:118`）。两条规则不一样，且分散在不同层。后续新增任何 mock 后端都应抽 `pkg/mockmode` 或 config 上挂统一方法，避免某些环境（如 staging Env=production 但 AppID 留空）行为漂移。
- **`keysToBind` 漏绑等于环境变量失效**。`internal/infrastructure/config/config.go` 的 `keysToBind` 数组每加一层嵌套 key 都必须显式追加，否则 Viper 的 `BindEnv` 不会生效。Batch 3 补了 `cors.allowed_origins` 和 `payment.wechat.allow_mock` 两个漏网之鱼——这已经是第二次踩到了。新增任何 `mapstructure` 字段，PR 检查表里加一条"是否同步加到 keysToBind"。
- **`clause.Locking{Strength: "UPDATE"}` + `First` 在记录不存在时不会等待**。`commercial_repository.go:897` 的 SELECT FOR UPDATE 紧跟 `gorm.ErrRecordNotFound` 分支处理订单不存在场景；Postgres 行为是无行可锁则立即返 not found，而不是阻塞。意味着 SELECT FOR UPDATE 后必须显式分类 not-found vs 真错误，**不能**只 `if err != nil { return err }` 一刀切，否则订单不存在会被当 500 回给微信触发重试。
- **三份 yaml 不是完全同构，按 cmd 需要裁剪**。`configs/config-brand.yaml` 既有 `sms` 又有 `payment.wechat`；`configs/config-admin.yaml` 只有 `payment`；`configs/config-app.yaml` 两者都没有。这是有意的——C 端不发短信也不收回调。后续加 service 配置时不要无脑三份都加，遵循"哪个 cmd 用就加哪份"的原则，避免给运维制造误导性的环境变量。
- **CallbackLog 两阶段写入便于排障**。`ProcessWeChatCallback` happy path 是先 `Create(callbackLog)` 拿 ID，再 `Update` 把 `transaction_id`、`status=processed`、`processed_at` 回填（见 `commercial_repository.go:597-616`）。比"事务末尾一次性写"好处是：如果后续 Transaction/Subscription 创建失败回滚，CallbackLog 也跟着回滚，但中间状态在 trace/log 里可观察；同时 `transaction_id` 字段外键依赖建立后再回填，避免循环依赖。
- **金额精度自实现要小心截断方向**。`parseAmountToFen` (`commercial_repository.go:1297`) 处理 "99.999" 时直接取前两位小数 → 9999 分（截断，非四舍五入）。当前业务金额都是两位小数无问题，但任何调用方传三位小数都会静默丢精度。如果未来对接其他支付渠道（如美元、日元零位小数），建议升级为 `shopspring/decimal` 或抽 `pkg/money` 加 round-half-even。

## 2026-06-06 Batch 4 patterns

- **GET 读路径里 self-heal 真实状态是合法模式，但要有写门槛**。`onboarding/service.go:60-92` 在 `GetOnboardingStatus` 里通过 `EnsureStepCompleted` 把计算出的 completed 持久化回表 — 避免下游审计/分析直接读 `brand_onboarding_steps` 看到永远的 NULL。**前提**：upsert 必须在 SQL 层用 `ON CONFLICT ... WHERE status NOT IN ('skipped','completed')`（见 `onboarding_repository.go:147-149`）保证幂等且不踩用户主动 skip / 不覆盖首次完成时间。后续 Staff / Course 状态机若有"前端打开页面时再校准"诉求，复用此模式时**禁止**省略 WHERE 子句，否则会变成 GET 写惊群。
- **8 步 step-driven 聚合架构可直接复用到 Staff/Course/Entitlement**。`onboarding/service.go` 的三件套——(1) `StepKey` 枚举 + `AllSteps()` 顺序、(2) `CountsByStep` 一次性 7 表 COUNT、(3) `computeStepView` switch 派生 status——形成了一套"资源表 COUNT → 派生 step 状态"的范式。后续任何"状态由多张子表实时聚合"的页面（会员卡/排课/教练日历）都可以照搬，但应优先解决 `FEATURE_REQUESTS.md` 提到的 step enum 表格化，否则 switch 与 `AllSteps()`/`IsSkippable()` 三处维护。
- **subscription 配额守卫的"事务内 SELECT FOR UPDATE → COUNT → INSERT"三段式模板**。`location_repository.go:34-89` 形成了模板：锁 active subscription（用 `grace_ends_at` / `expires_at` 双窗口判断，**不能**只看 `status='active'`，review #2）→ COUNT 当前资源（排除软删但**不**排除 inactive，防 disable→腾位 hack）→ 超限抛 `QUOTA_EXCEEDED` 带 `current`/`max` Details。Staff / Learner / Course 即将复制此结构，应**先**抽到 `commercial/subscription_guard.go`（见 FR），否则四处会各自漂移 `grace_ends_at` 处理或软删条件。
- **`AppError.Details` 取代 ad-hoc `gin.H` envelope**。`pkg/errors/error.go:62-78` + `pkg/response/response.go:49-66`：任何带额外数据的 4xx（quota current/max、字段错误清单、重试 after）都应 `apperr.NewAppError(...).WithDetails(map[string]any{...})`，由 `response.Error` 统一序列化进 `Response.Data`。**不要**再回到 handler 里 `c.JSON(409, gin.H{...})` 的 hack——会绕过 i18n 和 envelope，前端解析路径分裂。
- **`ALTER TABLE ... ADD COLUMN IF NOT EXISTS` 是开发期增量列的安全姿势**。`migrations/000004_brand_profile_extras.up.sql` 用 `IF NOT EXISTS` 防止本地反复跑 up/down 时的 "column already exists" 中断。**但**该 migration 在开发机上从未应用——下一批 Staff/Course 之前需要先解决 Makefile 中写死的 `postgres://postgres:postgres@...` 凭据问题（见 ERRORS.md 同日条目）。
- **code-review 双轨：阻塞修复立即提交 + 非阻塞挂 FEATURE_REQUESTS**。Batch 4 的 7 项 review 修复（onboarding 原子事务、quota expires_at、completed 持久化、status 过滤、skip 清 completed_at、AppError.Details、skipStep 不吞 bind 错）全部当批次合入；同时 11 项偏架构/性能/枚举的发现转入 `FEATURE_REQUESTS.md:9-23`。**约定**：post-impl review 输出必须二分——"现在合"对应 commit `fix(batch-N):`；"留到下一批"对应 FR 条目并标明 review 编号或场景，避免堆在脑里下批漂移。
- **白名单 PATCH 的字段隔离应在 handler binding 层就闭环**。`profile_handler.go:38-44` 的 `patchProfileBody` 只声明 5 个白名单字段（不含 `name`/`contact_phone`/`contact_name`），即便客户端发了也被 Gin 直接丢弃；service 层无需再做防御性删除。后续 Staff / Learner 的 PATCH 一律遵循"binding struct 即白名单契约"，不要在 service 里二次过滤——容易和 binding 漂移。
