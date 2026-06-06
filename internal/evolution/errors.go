package evolution

import "fmt"

type ErrorCode string

const (
	ErrValidation        ErrorCode = "EVOLUTION_VALIDATION_ERROR"
	ErrInvalidTransition ErrorCode = "EVOLUTION_INVALID_TRANSITION"
	ErrNotEligible       ErrorCode = "EVOLUTION_NOT_ELIGIBLE"
	ErrRepository        ErrorCode = "EVOLUTION_REPOSITORY_ERROR"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func validationError(message string) error {
	return &Error{Code: ErrValidation, Message: message}
}
