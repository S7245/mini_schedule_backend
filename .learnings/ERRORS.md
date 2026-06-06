## 2026-06-06 payment_callback_logs JSONB NOT NULL violated

**症状**：`POST /api/v1/public/payment/callback` 返 500。日志：
```
ERROR: null value in column "headers" of relation "payment_callback_logs" violates not-null constraint (SQLSTATE 23502)
```

**根因**：`PaymentCallbackLogModel.Headers` / `Payload` 字段是 `[]byte`，agent 实现未赋值 → GORM 把 nil 切片发为 SQL NULL（**不会**触发 column DEFAULT），列约束是 `JSONB NOT NULL DEFAULT '{}'` → 直接 23502。

**修复**：`commercial_models.go` 给 `PaymentCallbackLogModel` 加 `BeforeCreate(*gorm.DB) error` hook，nil 时填 `[]byte("{}")`。

**通用化教训**：GORM `[]byte` 字段映射到 PG JSONB 列时，应用层未显式赋值的情况都需要兜底；不要指望 DB DEFAULT 救场。其他可能受影响的列见 `commercial_models.go` 中含 `gorm:"type:jsonb"` 的字段。