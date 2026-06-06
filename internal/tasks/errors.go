package tasks

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeValidation        ErrorCode = "TASK_VALIDATION_ERROR"
	CodeNotFound          ErrorCode = "TASK_NOT_FOUND"
	CodeForbidden         ErrorCode = "TASK_FORBIDDEN"
	CodeVersionConflict   ErrorCode = "TASK_VERSION_CONFLICT"
	CodeInvalidTransition ErrorCode = "TASK_INVALID_TRANSITION"
	CodeAlreadyClaimed    ErrorCode = "TASK_ALREADY_CLAIMED"
	CodeVerification      ErrorCode = "TASK_VERIFICATION_REQUIRED"
	CodeRepository        ErrorCode = "TASK_REPOSITORY_ERROR"
	CodeAudit             ErrorCode = "TASK_AUDIT_ERROR"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

func IsCode(err error, code ErrorCode) bool {
	var taskErr *Error
	return errors.As(err, &taskErr) && taskErr.Code == code
}

func taskError(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
