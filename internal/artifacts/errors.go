package artifacts

import "fmt"

type ErrorCode string

const (
	ErrValidation             ErrorCode = "ARTIFACT_VALIDATION_ERROR"
	ErrNotFound               ErrorCode = "ARTIFACT_NOT_FOUND"
	ErrDeliveryNotFound       ErrorCode = "ARTIFACT_DELIVERY_NOT_FOUND"
	ErrUploadTokenInvalid     ErrorCode = "ARTIFACT_UPLOAD_TOKEN_INVALID"
	ErrUploadTokenExpired     ErrorCode = "ARTIFACT_UPLOAD_TOKEN_EXPIRED"
	ErrUploadAlreadyUsed      ErrorCode = "ARTIFACT_UPLOAD_ALREADY_USED"
	ErrDeliveryTokenInvalid   ErrorCode = "ARTIFACT_DELIVERY_TOKEN_INVALID"
	ErrDeliveryTokenExpired   ErrorCode = "ARTIFACT_DELIVERY_TOKEN_EXPIRED"
	ErrDeliveryDeviceMismatch ErrorCode = "ARTIFACT_DELIVERY_DEVICE_MISMATCH"
	ErrInvalidState           ErrorCode = "ARTIFACT_INVALID_STATE"
	ErrTooLarge               ErrorCode = "ARTIFACT_TOO_LARGE"
	ErrTargetKeyUnavailable   ErrorCode = "ARTIFACT_TARGET_KEY_UNAVAILABLE"
	ErrConflict               ErrorCode = "ARTIFACT_CONFLICT"
	ErrFetchNotFound          ErrorCode = "ARTIFACT_FETCH_NOT_FOUND"
	ErrFetchTokenInvalid      ErrorCode = "ARTIFACT_FETCH_TOKEN_INVALID"
	ErrFetchTokenExpired      ErrorCode = "ARTIFACT_FETCH_TOKEN_EXPIRED"
	ErrFetchDeviceMismatch    ErrorCode = "ARTIFACT_FETCH_DEVICE_MISMATCH"
)

type Error struct {
	Code    ErrorCode
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func domainError(code ErrorCode, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
