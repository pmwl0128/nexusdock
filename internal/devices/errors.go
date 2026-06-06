package devices

import "fmt"

// ErrorCode is stable and suitable for mapping to the public Nexus error contract.
type ErrorCode string

const (
	ErrValidation             ErrorCode = "VALIDATION_ERROR"
	ErrDeviceNotFound         ErrorCode = "DEVICE_NOT_FOUND"
	ErrDeviceAlreadyExists    ErrorCode = "DEVICE_ALREADY_EXISTS"
	ErrDeviceNotApproved      ErrorCode = "DEVICE_NOT_APPROVED"
	ErrDeviceRevoked          ErrorCode = "DEVICE_REVOKED"
	ErrDeviceTokenInvalid     ErrorCode = "DEVICE_TOKEN_INVALID"
	ErrEnrollmentTokenInvalid ErrorCode = "ENROLLMENT_TOKEN_INVALID"
	ErrEnrollmentTokenExpired ErrorCode = "ENROLLMENT_TOKEN_EXPIRED"
	ErrEnrollmentTokenUsed    ErrorCode = "ENROLLMENT_TOKEN_USED"
	ErrPolicyDenied           ErrorCode = "DEVICE_POLICY_DENIED"
	ErrVersionConflict        ErrorCode = "VERSION_CONFLICT"
)

// Error keeps domain failures machine-readable without importing the public DTO package.
type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func domainError(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsCode reports whether err is a devices domain error with code.
func IsCode(err error, code ErrorCode) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			return e.Code == code
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}
