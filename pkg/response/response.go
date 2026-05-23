package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperr "github.com/zkw/mini-schedule/backend/pkg/errors"
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
		Message: "success",
		Data:    data,
	})
}

// SuccessMessage 成功响应（仅消息）
func SuccessMessage(c *gin.Context, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: message,
	})
}

// SuccessNoData 成功响应（无数据）
func SuccessNoData(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code:    "OK",
		Message: "success",
	})
}

// ErrInvalidRequest 快捷构造无效请求错误
func ErrInvalidRequest(message string) error {
	return apperr.ErrBadRequest(message)
}

// Error 错误响应（从 AppError 转换）
func Error(c *gin.Context, err error) {
	if appErr := apperr.GetAppError(err); appErr != nil {
		c.JSON(appErr.HTTPStatus, Response{
			Code:    string(appErr.Code),
			Message: appErr.Message,
		})
		return
	}
	// 非 AppError，当作内部错误
	c.JSON(http.StatusInternalServerError, Response{
		Code:    string(apperr.ErrInternalServer),
		Message: "internal server error",
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
		Message: "success",
		Data: PageData{
			Items:     items,
			Total:     total,
			Page:      page,
			PageSize:  pageSize,
			TotalPage: totalPage,
		},
	})
}
