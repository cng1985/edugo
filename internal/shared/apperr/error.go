package apperr

import "errors"

// Error 是跨模块稳定错误。调用方只依赖 ErrorCode，不解析 Msg。
type Error struct {
	Status    int
	ErrorCode string
	Msg       string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.ErrorCode
}

func New(status int, code, msg string) *Error {
	return &Error{Status: status, ErrorCode: code, Msg: msg}
}

func Invalid(msg string) *Error      { return New(400, "INVALID_ARGUMENT", msg) }
func Unauthorized(msg string) *Error { return New(401, "UNAUTHENTICATED", msg) }
func Forbidden(msg string) *Error    { return New(403, "FORBIDDEN", msg) }
func NotFound(msg string) *Error     { return New(404, "NOT_FOUND", msg) }
func Conflict(code, msg string) *Error {
	return New(409, code, msg)
}
func TooLarge(msg string) *Error    { return New(413, "PAYLOAD_TOO_LARGE", msg) }
func Unsupported(msg string) *Error { return New(415, "UNSUPPORTED_MEDIA_TYPE", msg) }
func RateLimited(msg string) *Error { return New(429, "RATE_LIMITED", msg) }
func Internal(msg string) *Error    { return New(500, "INTERNAL", msg) }
func Unavailable(code, msg string) *Error {
	return New(503, code, msg)
}

func As(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal("内部错误")
}

func IsCode(err error, code string) bool {
	e := As(err)
	return e != nil && e.ErrorCode == code && err != nil && errors.As(err, &e)
}
