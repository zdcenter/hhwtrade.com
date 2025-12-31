package domain

import "errors"

// 定义通用业务错误
var (
	ErrNotFound          = errors.New("resource not found")
	ErrAlreadyExists     = errors.New("resource already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrInternalError     = errors.New("internal error")
	ErrOrderTerminal     = errors.New("order already in terminal state")
	ErrSubscriptionFailed = errors.New("subscription failed")
)

// AppError 应用错误，包含错误码和消息
type AppError struct {
	Code    int    // HTTP 状态码
	Message string // 用户友好的错误消息
	Err     error  // 原始错误
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// 创建常见错误的便捷函数
func NewNotFoundError(msg string) *AppError {
	return &AppError{Code: 404, Message: msg, Err: ErrNotFound}
}

func NewBadRequestError(msg string) *AppError {
	return &AppError{Code: 400, Message: msg, Err: ErrInvalidInput}
}

func NewInternalError(msg string, err error) *AppError {
	return &AppError{Code: 500, Message: msg, Err: err}
}

func NewConflictError(msg string) *AppError {
	return &AppError{Code: 409, Message: msg, Err: ErrAlreadyExists}
}
