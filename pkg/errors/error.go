package errors

import (
	"fmt"
	"net/http"
)

// ErrorCode 字符串错误码，前端/移动端调试友好，自解释
type ErrorCode string

const (
	// 通用错误
	ErrInternalServer   ErrorCode = "INTERNAL_SERVER_ERROR"
	ErrInvalidRequest   ErrorCode = "INVALID_REQUEST"
	ErrUnauthorized     ErrorCode = "UNAUTHORIZED"
	ErrForbidden        ErrorCode = "FORBIDDEN"
	ErrNotFound         ErrorCode = "NOT_FOUND"
	ErrTooManyRequests  ErrorCode = "TOO_MANY_REQUESTS"

	// 认证错误 (AUTH_*)
	ErrInvalidToken     ErrorCode = "AUTH_INVALID_TOKEN"
	ErrTokenExpired     ErrorCode = "AUTH_TOKEN_EXPIRED"
	ErrInvalidCredentials ErrorCode = "AUTH_INVALID_CREDENTIALS"
	ErrAccountDisabled  ErrorCode = "AUTH_ACCOUNT_DISABLED"

	// 品牌相关 (BRAND_*)
	ErrBrandNotFound    ErrorCode = "BRAND_NOT_FOUND"
	ErrBrandExists      ErrorCode = "BRAND_EXISTS"
	ErrBrandDisabled    ErrorCode = "BRAND_DISABLED"

	// 用户相关 (USER_*)
	ErrUserNotFound     ErrorCode = "USER_NOT_FOUND"
	ErrUserExists       ErrorCode = "USER_EXISTS"
	ErrUserDisabled     ErrorCode = "USER_DISABLED"

	// 课程相关 (COURSE_*)
	ErrCourseNotFound   ErrorCode = "COURSE_NOT_FOUND"
	ErrCourseDisabled   ErrorCode = "COURSE_DISABLED"

	// 训练记录相关 (TRAINING_*)
	ErrTrainingNotFound ErrorCode = "TRAINING_NOT_FOUND"
)

// AppError 自定义错误类型，包含业务错误码、用户提示消息和 HTTP 状态码
type AppError struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	HTTPStatus int       `json:"-"`
	Err        error     `json:"-"` // 内部错误，不暴露给前端
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError 创建 AppError
func NewAppError(code ErrorCode, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// NewAppErrorF 创建带内部错误的 AppError
func NewAppErrorF(code ErrorCode, message string, httpStatus int, err error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        err,
	}
}

// IsAppError 判断是否为 AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return err != nil && errAs(err, &appErr)
}

// GetAppError 从 error 中提取 AppError
func GetAppError(err error) *AppError {
	var appErr *AppError
	if errAs(err, &appErr) {
		return appErr
	}
	return nil
}

// ToHTTPStatus 将 AppError 转换为 HTTP 状态码，未映射的默认 500
func ToHTTPStatus(err error) int {
	if appErr := GetAppError(err); appErr != nil {
		return appErr.HTTPStatus
	}
	return http.StatusInternalServerError
}

// 快捷构造函数

func ErrInternal(message string) *AppError {
	return NewAppError(ErrInternalServer, message, http.StatusInternalServerError)
}

func ErrInternalF(message string, err error) *AppError {
	return NewAppErrorF(ErrInternalServer, message, http.StatusInternalServerError, err)
}

func ErrBadRequest(message string) *AppError {
	return NewAppError(ErrInvalidRequest, message, http.StatusBadRequest)
}

func ErrUnauthorizedF(message string) *AppError {
	return NewAppError(ErrUnauthorized, message, http.StatusUnauthorized)
}

func ErrForbiddenF(message string) *AppError {
	return NewAppError(ErrForbidden, message, http.StatusForbidden)
}

func ErrNotFoundF(code ErrorCode, message string) *AppError {
	return NewAppError(code, message, http.StatusNotFound)
}

// errAs 是 errors.As 的封装，避免循环导入
func errAs(err error, target interface{}) bool {
	if err == nil {
		return false
	}
	switch v := err.(type) {
	case *AppError:
		if t, ok := target.(**AppError); ok {
			*t = v
			return true
		}
	}
	return false
}
