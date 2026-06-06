package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
	"github.com/zkw/mini-schedule/backend/pkg/i18n"
)

// Response 统一 API 响应结构
type Response struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: message(c, i18n.KeySuccess, "success"),
		Data:    data,
	})
}

// SuccessMessage 成功响应（仅消息）
func SuccessMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: localize(c, message, message),
	})
}

// SuccessNoData 成功响应（无数据）
func SuccessNoData(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: message(c, i18n.KeySuccess, "success"),
	})
}

// ErrInvalidRequest 快捷构造无效请求错误
func ErrInvalidRequest(message string) error {
	return apperr.ErrBadRequest(message)
}

// Error 错误响应（从 AppError 转换）
func Error(c *gin.Context, err error) {
	if appErr := apperr.GetAppError(err); appErr != nil {
		resp := Response{
			Code:    string(appErr.Code),
			Message: errorMessage(c, appErr),
		}
		if len(appErr.Details) > 0 {
			resp.Data = appErr.Details
		}
		c.JSON(appErr.HTTPStatus, resp)
		return
	}
	// 非 AppError，当作内部错误
	c.JSON(http.StatusInternalServerError, Response{
		Code:    string(apperr.ErrInternalServer),
		Message: message(c, string(apperr.ErrInternalServer), "internal server error"),
	})
}

// PageData 分页数据结构
type PageData struct {
	Items     interface{} `json:"items"`
	Total     int64       `json:"total"`
	Page      int         `json:"page"`
	PageSize  int         `json:"page_size"`
	TotalPage int         `json:"total_page"`
}

// SuccessPage 分页成功响应
func SuccessPage(c *gin.Context, items interface{}, total int64, page, pageSize int) {
	totalPage := int(total) / pageSize
	if int(total)%pageSize > 0 {
		totalPage++
	}
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: message(c, i18n.KeySuccess, "success"),
		Data: PageData{
			Items:     items,
			Total:     total,
			Page:      page,
			PageSize:  pageSize,
			TotalPage: totalPage,
		},
	})
}

func errorMessage(c *gin.Context, appErr *apperr.AppError) string {
	key := appErr.MessageKey
	if key == "" {
		key = appErr.Message
	}

	fallback := appErr.Message
	if fallback == "" {
		fallback = string(appErr.Code)
	}

	localized := localize(c, key, fallback)
	if localized != fallback || key == string(appErr.Code) {
		return localized
	}

	if appErr.Code != apperr.ErrInternalServer {
		return fallback
	}

	return message(c, string(appErr.Code), fallback)
}

func message(c *gin.Context, key, fallback string) string {
	return localize(c, key, fallback)
}

func localize(c *gin.Context, key, fallback string) string {
	var locale i18n.Locale
	if c != nil {
		locale = i18n.FromRequest(c.Request)
	}
	return i18n.Localize(locale, key, fallback)
}
