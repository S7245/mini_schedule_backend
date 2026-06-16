package errors

import (
	stderrors "errors"
	"fmt"
	"net/http"
)

// ErrorCode 字符串错误码，前端/移动端调试友好，自解释
type ErrorCode string

const (
	// 通用错误
	ErrInternalServer  ErrorCode = "INTERNAL_SERVER_ERROR"
	ErrInvalidRequest  ErrorCode = "INVALID_REQUEST"
	ErrUnauthorized    ErrorCode = "UNAUTHORIZED"
	ErrForbidden       ErrorCode = "FORBIDDEN"
	ErrNotFound        ErrorCode = "NOT_FOUND"
	ErrTooManyRequests ErrorCode = "TOO_MANY_REQUESTS"

	// 认证错误 (AUTH_*)
	ErrInvalidToken       ErrorCode = "AUTH_INVALID_TOKEN"
	ErrTokenExpired       ErrorCode = "AUTH_TOKEN_EXPIRED"
	ErrInvalidCredentials ErrorCode = "AUTH_INVALID_CREDENTIALS"
	ErrAccountDisabled    ErrorCode = "AUTH_ACCOUNT_DISABLED"

	// 品牌相关 (BRAND_*)
	ErrBrandNotFound ErrorCode = "BRAND_NOT_FOUND"
	ErrBrandExists   ErrorCode = "BRAND_EXISTS"
	ErrBrandDisabled ErrorCode = "BRAND_DISABLED"

	// 用户相关 (USER_*)
	ErrUserNotFound ErrorCode = "USER_NOT_FOUND"
	ErrUserExists   ErrorCode = "USER_EXISTS"
	ErrUserDisabled ErrorCode = "USER_DISABLED"

	// 课程相关 (COURSE_*)
	ErrCourseNotFound ErrorCode = "COURSE_NOT_FOUND"
	ErrCourseDisabled ErrorCode = "COURSE_DISABLED"

	// 训练记录相关 (TRAINING_*)
	ErrTrainingNotFound ErrorCode = "TRAINING_NOT_FOUND"

	// Batch 4 — 通用参数 / 品牌资料
	ErrInvalidParam        ErrorCode = "INVALID_PARAM"
	ErrBrandNotActive      ErrorCode = "BRAND_NOT_ACTIVE"
	ErrBrandCodeDuplicated ErrorCode = "BRAND_CODE_DUPLICATED"

	// Batch 4 — Onboarding
	ErrStepNotSkippable   ErrorCode = "STEP_NOT_SKIPPABLE"
	ErrInvalidStepKey     ErrorCode = "INVALID_STEP_KEY"
	ErrOnboardingNotReady ErrorCode = "ONBOARDING_NOT_READY"

	// Batch 4 — Location & subscription quota
	ErrLocationNameDuplicated ErrorCode = "LOCATION_NAME_DUPLICATED"
	ErrLocationNotFound       ErrorCode = "LOCATION_NOT_FOUND"
	ErrQuotaExceeded          ErrorCode = "QUOTA_EXCEEDED"
	ErrSubscriptionRestricted ErrorCode = "SUBSCRIPTION_RESTRICTED"
	// Batch 9 — 门店删除引用保护
	// ErrLocationInUse (HTTP 409)：删除门店时仍有 active 员工任职或门店级角色任职引用（镜像 Batch 7 ErrRoleInUse）。
	ErrLocationInUse ErrorCode = "LOCATION_IN_USE"

	// Batch 5 — Staff / Role / Instructor
	ErrStaffPhoneDuplicated      ErrorCode = "STAFF_PHONE_DUPLICATED"
	ErrStaffNotFound             ErrorCode = "STAFF_NOT_FOUND"
	ErrOwnerProtected            ErrorCode = "OWNER_PROTECTED"
	ErrRoleNotFound              ErrorCode = "ROLE_NOT_FOUND"
	ErrLocationAssignmentInvalid ErrorCode = "LOCATION_ASSIGNMENT_INVALID"
	ErrInstructorProfileNotFound ErrorCode = "INSTRUCTOR_PROFILE_NOT_FOUND"
	// review B8：并发 PUT instructor 撞 unique index 时使用，前端可据此提示"另一会话已建好"。
	ErrInstructorProfileConflict ErrorCode = "INSTRUCTOR_PROFILE_CONFLICT"

	// Batch 6 — RBAC enforcement
	// ErrPermissionDenied (HTTP 403)：service 层 RequirePermission 失败时返回，
	// Details 含 {required: "<code>", missing: ["<code>"]} 方便前端 toast + 排错。
	ErrPermissionDenied ErrorCode = "PERMISSION_DENIED"

	// Batch 7 — 品牌自定义角色 CRUD
	// ErrRoleIsSystem (HTTP 409)：试图改 / 删 is_system=TRUE 的系统角色。
	ErrRoleIsSystem ErrorCode = "ROLE_IS_SYSTEM"
	// ErrRoleInUse (HTTP 409)：删除时仍有 active 任职引用该角色（A4）。
	ErrRoleInUse ErrorCode = "ROLE_IN_USE"
	// ErrRolePermissionExceedsActor (HTTP 403)：创建 / 编辑角色时勾选的权限超出
	// actor 自身有效权限集（B1，非 owner）。
	ErrRolePermissionExceedsActor ErrorCode = "ROLE_PERMISSION_EXCEEDS_ACTOR"
	// ErrRoleCodeDuplicated (HTTP 409)：(brand_id, code) 冲突兜底（D3）。
	ErrRoleCodeDuplicated ErrorCode = "ROLE_CODE_DUPLICATED"

	// Batch 11 — CourseCategory / CourseTemplate / ClassSession。
	// 注意：COURSE_NOT_FOUND 已在上方「课程相关」声明（ErrCourseNotFound, 404），此处复用，不重复定义。
	// ErrCategoryNotFound (404)：category_ids 含非本 brand active 分类。
	ErrCategoryNotFound ErrorCode = "CATEGORY_NOT_FOUND"
	// ErrCategoryNameDuplicated (409)：course_categories(brand_id,name) 唯一约束冲突。
	ErrCategoryNameDuplicated ErrorCode = "CATEGORY_NAME_DUPLICATED"
	// ErrCourseNotActive (409)：排课时所选课程模板非 published。
	ErrCourseNotActive ErrorCode = "COURSE_NOT_ACTIVE"
	// ErrCourseInUse (409)：删除课程模板时仍有 scheduled/in_progress 场次引用。
	ErrCourseInUse ErrorCode = "COURSE_IN_USE"
	// ErrCourseLocationUnavailable (409)：课程在该门店不可用（course_location_availability）。
	ErrCourseLocationUnavailable ErrorCode = "COURSE_LOCATION_UNAVAILABLE"
	// ErrSessionNotFound (404)：场次不存在或越权。
	ErrSessionNotFound ErrorCode = "SESSION_NOT_FOUND"
	// ErrSessionTimeInvalid (400)：ends_at<=starts_at 或 starts_at 已过去。
	ErrSessionTimeInvalid ErrorCode = "SESSION_TIME_INVALID"
	// ErrSessionInstructorConflict (409)：教练同时段重叠（DB EXCLUDE 23P01）。
	ErrSessionInstructorConflict ErrorCode = "SESSION_INSTRUCTOR_CONFLICT"
	// ErrSessionCancelNotAllowed (409)：仅 scheduled/in_progress 可取消。
	ErrSessionCancelNotAllowed ErrorCode = "SESSION_CANCEL_NOT_ALLOWED"
	// ErrInstructorNotSchedulable (409)：教练 is_schedulable=false 或非 active。
	ErrInstructorNotSchedulable ErrorCode = "INSTRUCTOR_NOT_SCHEDULABLE"

	// Location Resource 资源管理 (Batch 12a)
	// ErrResourceNotFound (404)：资源不存在或越权。
	ErrResourceNotFound ErrorCode = "RESOURCE_NOT_FOUND"
	// ErrResourceNameDuplicated (409)：同门店资源重名（unique(location_id,name) where not deleted）。
	ErrResourceNameDuplicated ErrorCode = "RESOURCE_NAME_DUPLICATED"
	// ErrResourceInUse (409)：删除资源时仍被 scheduled/in_progress 场次或 active 循环排课引用。
	ErrResourceInUse ErrorCode = "RESOURCE_IN_USE"
	// ErrResourceNotAvailable (409)：排课绑定的资源已停用 / 软删 / 跨门店。
	ErrResourceNotAvailable ErrorCode = "RESOURCE_NOT_AVAILABLE"
	// ErrSessionResourceConflict (409)：同一资源同一时段重叠（DB EXCLUDE class_sessions_resource_no_overlap，23P01）。
	ErrSessionResourceConflict ErrorCode = "SESSION_RESOURCE_CONFLICT"

	// 循环排课 (Batch 12b)
	// ErrRecurringNotFound (404)：循环排课不存在或越权。
	ErrRecurringNotFound ErrorCode = "RECURRING_NOT_FOUND"
	// ErrRecurringAllConflict (409)：全部 occurrence 冲突，未生成任何场次（body 带 skipped 清单）。
	ErrRecurringAllConflict ErrorCode = "RECURRING_ALL_CONFLICT"
	// ErrRecurringCancelNotAllowed (409)：仅 active 状态的循环排课可取消。
	ErrRecurringCancelNotAllowed ErrorCode = "RECURRING_CANCEL_NOT_ALLOWED"
)

// AppError 自定义错误类型，包含业务错误码、用户提示消息和 HTTP 状态码
type AppError struct {
	Code       ErrorCode      `json:"code"`
	MessageKey string         `json:"-"`
	Message    string         `json:"message"`
	HTTPStatus int            `json:"-"`
	Err        error          `json:"-"` // 内部错误，不暴露给前端
	Details    map[string]any `json:"-"` // 业务级附加数据（如 quota current/max），由 response.Error 统一序列化进 Response.Data
}

// WithDetails 链式给 AppError 挂额外的 data，供前端展示。
func (e *AppError) WithDetails(details map[string]any) *AppError {
	if e == nil {
		return nil
	}
	e.Details = details
	return e
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
		MessageKey: message,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// NewAppErrorF 创建带内部错误的 AppError
func NewAppErrorF(code ErrorCode, message string, httpStatus int, err error) *AppError {
	return &AppError{
		Code:       code,
		MessageKey: message,
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        err,
	}
}

// NewAppErrorWithKey 创建带翻译 key 的 AppError，message 作为默认回退文案
func NewAppErrorWithKey(code ErrorCode, messageKey, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		MessageKey: messageKey,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// IsAppError 判断是否为 AppError
func IsAppError(err error) bool {
	var appErr *AppError
	return err != nil && stderrors.As(err, &appErr)
}

// GetAppError 从 error 中提取 AppError
func GetAppError(err error) *AppError {
	var appErr *AppError
	if stderrors.As(err, &appErr) {
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
