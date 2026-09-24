package apperrors

import "errors"

var (
	ErrNotFound       = errors.New("resource not found")
	ErrUnauthorized   = errors.New("unauthorized access")
	ErrInvalidInput   = errors.New("invalid input")
	ErrInternal       = errors.New("internal server error")
	ErrConflict       = errors.New("identity conflict")
	ErrAccountDeleted = errors.New("account has been deleted")
)
