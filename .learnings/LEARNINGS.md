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
