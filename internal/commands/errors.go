package commands

import "fmt"

type ErrorCode string

const (
	ErrValidation          ErrorCode = "VALIDATION_ERROR"
	ErrCommandNotFound     ErrorCode = "COMMAND_NOT_FOUND"
	ErrCommandTypeDenied   ErrorCode = "COMMAND_TYPE_DENIED"
	ErrCommandExpired      ErrorCode = "COMMAND_EXPIRED"
	ErrCommandCancelled    ErrorCode = "COMMAND_CANCELLED"
	ErrCommandTerminal     ErrorCode = "COMMAND_TERMINAL"
	ErrCommandNotLeaseable ErrorCode = "COMMAND_NOT_LEASEABLE"
	ErrLeaseNotFound       ErrorCode = "LEASE_NOT_FOUND"
	ErrLeaseMismatch       ErrorCode = "LEASE_MISMATCH"
	ErrLeaseExpired        ErrorCode = "LEASE_EXPIRED"
	ErrInvalidTransition   ErrorCode = "INVALID_COMMAND_TRANSITION"
	ErrVersionConflict     ErrorCode = "VERSION_CONFLICT"
)

type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func commandError(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func IsCode(err error, code ErrorCode) bool {
	for err != nil {
		if typed, ok := err.(*Error); ok {
			return typed.Code == code
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
