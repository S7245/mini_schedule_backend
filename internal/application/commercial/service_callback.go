package commercial

import (
	"context"
	"time"

	"github.com/zkw/mini-schedule/backend/internal/domain/commercial"
)

// ProcessWeChatPaymentCallbackInput 是上层 HTTP 解析后传入的原始 headers + body。
type ProcessWeChatPaymentCallbackInput struct {
	Headers map[string]string
	Body    []byte
}

// ProcessWeChatPaymentCallback 是微信支付回调的应用层入口。
//
// 步骤：
//  1. 委托 adapter 做 mock / 真实验签 + 解密
//  2. 失败 → 写一条 CallbackLog(failed) 后向上返回 401（具体响应由 handler 决定）
//  3. 调 repository.ProcessWeChatCallback 执行状态机推进
//  4. 透传 repo 结果给 handler
func (s *Service) ProcessWeChatPaymentCallback(
	ctx context.Context,
	input ProcessWeChatPaymentCallbackInput,
) (*commercial.ProcessWeChatCallbackResult, error) {
	now := time.Now()

	notif, err := s.wechatAdapter.VerifyAndDecrypt(ctx, input.Headers, input.Body, now)
	if err != nil {
		// 验签 / 解密 / 时间戳失败：单独写一条 CallbackLog 留痕，便于排查。
		// 注意：此时尚未拿到 order_id / brand_id，全部为 nil。
		s.writeCallbackLogSilently(ctx, commercial.PaymentCallbackLog{
			PaymentChannel: commercial.PaymentChannelWeChat,
			Status:         commercial.PaymentCallbackLogStatusFailed,
			ErrorMessage:   truncate(err.Error(), 900),
		})
		return nil, err
	}

	repoInput := commercial.ProcessWeChatCallbackInput{
		OutTradeNo:        notif.OutTradeNo,
		ThirdPartyTradeNo: notif.TransactionID,
		Amount:            notif.AmountTotal,
		Currency:          notif.Currency,
		TradeState:        notif.TradeState,
		PaymentChannel:    commercial.PaymentChannelWeChat,
		CallbackRequestID: notif.CallbackRequestID,
		SuccessTime:       notif.SuccessTime,
		ReceivedAt:        now,
		RawPayload:        notif.RawPayload,
	}

	result, err := s.repo.ProcessWeChatCallback(ctx, repoInput)
	if err != nil {
		// 仓储事务回滚：在事务外补写一条 failed 日志（带 out_trade_no），方便后续运营补偿。
		s.writeCallbackLogSilently(ctx, commercial.PaymentCallbackLog{
			PaymentChannel:    commercial.PaymentChannelWeChat,
			OutTradeNo:        notif.OutTradeNo,
			ThirdPartyTradeNo: notif.TransactionID,
			CallbackRequestID: notif.CallbackRequestID,
			Status:            commercial.PaymentCallbackLogStatusFailed,
			ErrorMessage:      truncate(err.Error(), 900),
		})
		return nil, err
	}
	return result, nil
}

// writeCallbackLogSilently 尝试写一条 CallbackLog，写失败时只记录在内部日志，
// 不影响主流程的返回值（这是补偿性记录，不应影响 HTTP 响应码）。
func (s *Service) writeCallbackLogSilently(ctx context.Context, log commercial.PaymentCallbackLog) {
	if s.repo == nil {
		return
	}
	if log.PaymentChannel == "" {
		log.PaymentChannel = commercial.PaymentChannelWeChat
	}
	_ = s.repo.WritePaymentCallbackLog(ctx, log)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

