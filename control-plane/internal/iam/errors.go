package iam

import "fmt"

// ErrorKind 错误分类，供 API 层映射 HTTP 状态码。
type ErrorKind string

const (
	KindNotFound ErrorKind = "not_found"
	KindConflict ErrorKind = "conflict"
	KindInvalid  ErrorKind = "invalid"
)

// APIError 带错误分类的错误，便于 REST 层返回稳定结构。
type APIError struct {
	Kind ErrorKind
	Msg  string
}

func (e *APIError) Error() string { return e.Msg }

// NotFoundError 资源不存在。
type NotFoundError struct {
	Kind string // 资源类型：incident / operation / evidence ...
	ID   string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Kind, e.ID)
}

// ConflictError 资源已存在（幂等冲突）。
type ConflictError struct {
	Kind string
	ID   string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s %q already exists", e.Kind, e.ID)
}
