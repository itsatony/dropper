package dropper

import (
	"errors"
	"net/http"
)

// --- Sentinel errors ---
// Used with errors.Is() to match error categories without string comparison.

var (
	ErrPathTraversal  = errors.New(ErrMsgPathTraversal)
	ErrPathResolution = errors.New(ErrMsgPathResolution)
	ErrReadonlyMode   = errors.New(ErrMsgReadonlyMode)
	ErrExtNotAllowed  = errors.New(ErrMsgExtNotAllowed)
	ErrNotDirectory   = errors.New(ErrMsgNotDirectory)
	ErrNotFile        = errors.New(ErrMsgNotFile)
	ErrFileTooLarge   = errors.New(ErrMsgFileTooLarge)
	ErrFileExists     = errors.New(ErrMsgFileExists)
	ErrInvalidFilename = errors.New(ErrMsgInvalidFilename)
	ErrListDir        = errors.New(ErrMsgListDir)
	ErrCreateDir      = errors.New(ErrMsgCreateDir)
	ErrWriteFile      = errors.New(ErrMsgWriteFile)
	ErrTempFile       = errors.New(ErrMsgTempFile)
	ErrRenameFile     = errors.New(ErrMsgRenameFile)
	ErrBodyTooLarge   = errors.New(ErrMsgBodyTooLarge)
	ErrFileStat       = errors.New(ErrMsgFileStat)
)

// DropperError is a typed error that carries HTTP response metadata alongside
// the error. It supports errors.Is() matching via a sentinel error and
// errors.As() extraction for HTTP handler error mapping.
type DropperError struct {
	// Sentinel is the category error for errors.Is() matching.
	Sentinel error
	// StatusCode is the HTTP status code to return to the client.
	StatusCode int
	// Code is the machine-readable error code (e.g. "forbidden").
	Code string
	// SafeMsg is the client-safe message (never contains internal paths).
	SafeMsg string
	// Wrapped is the optional underlying error (for debugging/logging).
	Wrapped error
}

// Error returns the safe client message. Internal details from Wrapped are
// never exposed — use Unwrap() or log the error for debugging.
func (e *DropperError) Error() string {
	if e.Wrapped != nil {
		return e.SafeMsg + ": " + e.Wrapped.Error()
	}
	return e.SafeMsg
}

// Unwrap returns the sentinel error so errors.Is(err, ErrPathTraversal) works
// through arbitrary wrapping depth.
func (e *DropperError) Unwrap() error {
	return e.Sentinel
}

// NewDropperError creates a DropperError with full control over all fields.
func NewDropperError(sentinel error, statusCode int, code, safeMsg string, wrapped error) *DropperError {
	return &DropperError{
		Sentinel:   sentinel,
		StatusCode: statusCode,
		Code:       code,
		SafeMsg:    safeMsg,
		Wrapped:    wrapped,
	}
}

// --- Convenience constructors ---

// NewPathTraversalError returns a 403 Forbidden error for path jail violations.
func NewPathTraversalError() *DropperError {
	return NewDropperError(ErrPathTraversal, http.StatusForbidden, ErrCodeForbidden, ErrMsgPathTraversal, nil)
}

// NewPathResolutionError returns a 500 error for path resolution failures.
func NewPathResolutionError(wrapped error) *DropperError {
	return NewDropperError(ErrPathResolution, http.StatusInternalServerError, ErrCodeInternal, ErrMsgPathResolution, wrapped)
}

// NewReadonlyError returns a 403 Forbidden error for write attempts in readonly mode.
func NewReadonlyError() *DropperError {
	return NewDropperError(ErrReadonlyMode, http.StatusForbidden, ErrCodeReadonly, ErrMsgReadonlyMode, nil)
}

// NewExtNotAllowedError returns a 400 Bad Request error for rejected extensions.
func NewExtNotAllowedError() *DropperError {
	return NewDropperError(ErrExtNotAllowed, http.StatusBadRequest, ErrCodeExtNotAllowed, ErrMsgExtNotAllowed, nil)
}

// NewNotDirectoryError returns a 400 Bad Request error when a directory was expected.
func NewNotDirectoryError() *DropperError {
	return NewDropperError(ErrNotDirectory, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNotDirectory, nil)
}

// NewNotFileError returns a 400 Bad Request error when a file was expected.
func NewNotFileError() *DropperError {
	return NewDropperError(ErrNotFile, http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNotFile, nil)
}

// NewFileTooLargeError returns a 413 error when upload exceeds max size.
func NewFileTooLargeError() *DropperError {
	return NewDropperError(ErrFileTooLarge, http.StatusRequestEntityTooLarge, ErrCodeFileTooLarge, ErrMsgFileTooLarge, nil)
}

// NewFileExistsError returns a 409 error after collision retries are exhausted.
func NewFileExistsError() *DropperError {
	return NewDropperError(ErrFileExists, http.StatusConflict, ErrCodeInternal, ErrMsgFileExists, nil)
}

// NewListDirError returns a 500 error for directory listing failures.
func NewListDirError(wrapped error) *DropperError {
	return NewDropperError(ErrListDir, http.StatusInternalServerError, ErrCodeInternal, ErrMsgListDir, wrapped)
}

// NewCreateDirError returns a 500 error for directory creation failures.
func NewCreateDirError(wrapped error) *DropperError {
	return NewDropperError(ErrCreateDir, http.StatusInternalServerError, ErrCodeInternal, ErrMsgCreateDir, wrapped)
}

// NewWriteFileError returns a 500 error for file write failures.
func NewWriteFileError(wrapped error) *DropperError {
	return NewDropperError(ErrWriteFile, http.StatusInternalServerError, ErrCodeInternal, ErrMsgWriteFile, wrapped)
}

// NewTempFileError returns a 500 error for temp file creation failures.
func NewTempFileError(wrapped error) *DropperError {
	return NewDropperError(ErrTempFile, http.StatusInternalServerError, ErrCodeInternal, ErrMsgTempFile, wrapped)
}

// NewRenameFileError returns a 500 error for atomic rename failures.
func NewRenameFileError(wrapped error) *DropperError {
	return NewDropperError(ErrRenameFile, http.StatusInternalServerError, ErrCodeInternal, ErrMsgRenameFile, wrapped)
}

// NewFileStatError returns a 404 error for stat failures on expected files.
func NewFileStatError(wrapped error) *DropperError {
	return NewDropperError(ErrFileStat, http.StatusNotFound, ErrCodeNotFound, ErrMsgFileStat, wrapped)
}

// MapDropperError extracts HTTP response fields from a DropperError.
// Falls back to 500 Internal Server Error for non-DropperError types.
func MapDropperError(err error) (statusCode int, errCode string, safeMsg string) {
	var de *DropperError
	if errors.As(err, &de) {
		return de.StatusCode, de.Code, de.SafeMsg
	}
	return http.StatusInternalServerError, ErrCodeInternal, ErrMsgWriteFile
}

// SafeErrorMessage returns only the client-safe message from an error,
// stripping any internal path information.
func SafeErrorMessage(err error) string {
	_, _, safeMsg := MapDropperError(err)
	return safeMsg
}
