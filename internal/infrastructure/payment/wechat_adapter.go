package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
	"github.com/zkw/mini-schedule/backend/internal/infrastructure/config"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
)

const (
	headerSignature = "Wechatpay-Signature"
	headerTimestamp = "Wechatpay-Timestamp"
	headerNonce     = "Wechatpay-Nonce"
	headerSerial    = "Wechatpay-Serial"
	mockSignature   = "mock_signature"
)

// WeChatPaymentAdapter 封装微信支付 v3 回调验签与解密。
//
// 当前阶段只实现 mock 路径，真实证书 / RSA-SHA256 / AES-256-GCM 留 TODO。
type WeChatPaymentAdapter struct {
	cfg *config.Config
}

func NewWeChatPaymentAdapter(cfg *config.Config) *WeChatPaymentAdapter {
	return &WeChatPaymentAdapter{cfg: cfg}
}

// CallbackNotification 是验签 + 解密后回调载荷的中间结构。
//
// mock 模式下，请求体 JSON 必须满足如下字段：
//
//	{
//	  "out_trade_no":     "MSXXX",
//	  "transaction_id":   "wx_tx_001",        // 等价于微信 v3 的 resource.id
//	  "trade_state":      "SUCCESS|USERPAYING|CLOSED|PAYERROR|...",
//	  "amount":           {"total": 9900, "currency": "CNY"},
//	  "success_time":     "2026-06-04T10:30:45+08:00"  // optional, RFC3339
//	}
type CallbackNotification struct {
	OutTradeNo        string
	TransactionID     string
	TradeState        commercial.WeChatTradeState
	AmountTotal       int64 // 分
	Currency          string
	SuccessTime       *time.Time
	CallbackRequestID string // 用作幂等 + 重放追踪
	RawPayload        string
}

// rawNotification 描述 mock 路径下直接 unmarshal 的 JSON 结构。
type rawNotification struct {
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	Amount        struct {
		Total    int64  `json:"total"`
		Currency string `json:"currency"`
	} `json:"amount"`
	SuccessTime string `json:"success_time"`
	// 可选：v3 风格的顶层 id（事件 ID）
	EventID string `json:"id"`
}

// VerifyAndDecrypt 校验微信回调 headers + body，返回解密后的中间结构。
//
// mock 模式（cfg.Payment.WeChat.AllowMock == true 或 AppID 为空）：
//   - 必须有 Wechatpay-Signature / Wechatpay-Timestamp 头
//   - Wechatpay-Signature == "mock_signature" 视为通过
//   - 其他签名一律失败
//   - 时间戳必须落在 [now-5min, now+5min] 区间内
//   - body 必须是合法 JSON，结构同 rawNotification
//   - 不做 AES-GCM 解密
//
// 生产模式（AllowMock=false 且 AppID 非空）：
//   - TODO: 实现真正的 RSA-SHA256 验签 + AES-256-GCM 解密
//   - 当前阶段直接返回 errors.New(...)，避免误用
func (a *WeChatPaymentAdapter) VerifyAndDecrypt(
	ctx context.Context,
	headers map[string]string,
	body []byte,
	now time.Time,
) (*CallbackNotification, error) {
	_ = ctx

	signature := headerValue(headers, headerSignature)
	timestampStr := headerValue(headers, headerTimestamp)
	nonce := headerValue(headers, headerNonce)
	_ = nonce // mock 路径不做 nonce 校验

	if signature == "" {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "missing wechatpay signature header", 401)
	}
	if timestampStr == "" {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "missing wechatpay timestamp header", 401)
	}

	tsSec, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "invalid wechatpay timestamp header", 401)
	}
	ts := time.Unix(tsSec, 0)
	diff := now.Sub(ts)
	if diff < 0 {
		diff = -diff
	}
	if diff > commercial.PaymentCallbackReplayWindow {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "wechatpay timestamp out of replay window", 401)
	}

	allowMock := a.cfg != nil && (a.cfg.Payment.WeChat.AllowMock || a.cfg.Payment.WeChat.AppID == "")
	if !allowMock {
		// TODO(payments): 接入真实微信 v3 验签：
		//   1. 拼接待签字符串 = timestamp + "\n" + nonce + "\n" + body + "\n"
		//   2. 用 Wechatpay-Serial 选择对应平台证书公钥
		//   3. RSA-SHA256 验签 Wechatpay-Signature（base64 解码）
		//   4. AES-256-GCM 解密 resource.ciphertext，nonce/associated_data 取自 body
		//   5. 解析解密后的 JSON 为 CallbackNotification
		return nil, errors.New("real wechat signature verification not implemented yet")
	}

	// ---- mock 路径 ----
	if signature != mockSignature {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "invalid wechatpay signature", 401)
	}
	if len(body) == 0 {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "empty wechatpay callback body", 401)
	}

	var raw rawNotification
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, apperr.NewAppErrorF(apperr.ErrUnauthorized, "invalid wechatpay callback body", 401, err)
	}

	if raw.OutTradeNo == "" {
		return nil, apperr.NewAppError(apperr.ErrUnauthorized, "missing out_trade_no in callback body", 401)
	}

	notif := &CallbackNotification{
		OutTradeNo:    raw.OutTradeNo,
		TransactionID: raw.TransactionID,
		TradeState:    commercial.WeChatTradeState(strings.ToUpper(strings.TrimSpace(raw.TradeState))),
		AmountTotal:   raw.Amount.Total,
		Currency:      strings.ToUpper(strings.TrimSpace(raw.Amount.Currency)),
		RawPayload:    string(body),
	}

	// CallbackRequestID 优先用 body 内的 event id（微信 v3 的 resource.id / 顶层 id），
	// 否则退化为 transaction_id，便于幂等查重 / 调试。
	switch {
	case raw.EventID != "":
		notif.CallbackRequestID = raw.EventID
	case raw.TransactionID != "":
		notif.CallbackRequestID = raw.TransactionID
	default:
		notif.CallbackRequestID = fmt.Sprintf("%s:%d", raw.OutTradeNo, ts.Unix())
	}

	if notif.Currency == "" {
		notif.Currency = "CNY"
	}

	if raw.SuccessTime != "" {
		if t, err := time.Parse(time.RFC3339, raw.SuccessTime); err == nil {
			notif.SuccessTime = &t
		}
	}

	return notif, nil
}

func headerValue(headers map[string]string, key string) string {
	if v, ok := headers[key]; ok {
		return strings.TrimSpace(v)
	}
	// 兼容大小写差异：HTTP header 大小写不敏感
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
