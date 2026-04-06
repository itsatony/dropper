package dropper

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropperError_Error_ReturnsSafeMsg(t *testing.T) {
	de := NewPathTraversalError()
	assert.Equal(t, ErrMsgPathTraversal, de.Error())
}

func TestDropperError_Error_IncludesWrapped(t *testing.T) {
	inner := fmt.Errorf("open /tmp/secret: permission denied")
	de := NewWriteFileError(inner)

	// Error() includes the wrapped error for logging, but SafeMsg is clean.
	assert.Contains(t, de.Error(), ErrMsgWriteFile)
	assert.Contains(t, de.Error(), "permission denied")
	assert.Equal(t, ErrMsgWriteFile, de.SafeMsg)
}

func TestDropperError_Is_SentinelMatch(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		sentinel error
		want     bool
	}{
		{"path too long matches", NewPathTooLongError(), ErrPathTooLong, true},
		{"path traversal matches", NewPathTraversalError(), ErrPathTraversal, true},
		{"readonly matches", NewReadonlyError(), ErrReadonlyMode, true},
		{"ext not allowed matches", NewExtNotAllowedError(), ErrExtNotAllowed, true},
		{"not directory matches", NewNotDirectoryError(), ErrNotDirectory, true},
		{"not file matches", NewNotFileError(), ErrNotFile, true},
		{"file too large matches", NewFileTooLargeError(), ErrFileTooLarge, true},
		{"file exists matches", NewFileExistsError(), ErrFileExists, true},
		{"write file matches", NewWriteFileError(nil), ErrWriteFile, true},
		{"temp file matches", NewTempFileError(nil), ErrTempFile, true},
		{"rename file matches", NewRenameFileError(nil), ErrRenameFile, true},
		{"list dir matches", NewListDirError(nil), ErrListDir, true},
		{"create dir matches", NewCreateDirError(nil), ErrCreateDir, true},
		{"file stat matches", NewFileStatError(nil), ErrFileStat, true},
		{"path resolution matches", NewPathResolutionError(nil), ErrPathResolution, true},
		{"invalid relpath matches", NewInvalidRelPathError(), ErrInvalidRelPath, true},
		{"csrf rejected matches", NewCSRFError(), ErrCSRFRejected, true},
		// Negative cases.
		{"traversal is not readonly", NewPathTraversalError(), ErrReadonlyMode, false},
		{"readonly is not traversal", NewReadonlyError(), ErrPathTraversal, false},
		{"write is not temp", NewWriteFileError(nil), ErrTempFile, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, errors.Is(tt.err, tt.sentinel))
		})
	}
}

func TestDropperError_Is_ThroughFmtErrorfWrap(t *testing.T) {
	// Wrapping a DropperError with fmt.Errorf should still allow errors.Is matching.
	original := NewPathTraversalError()
	wrapped := fmt.Errorf("handler context: %w", original)

	assert.True(t, errors.Is(wrapped, ErrPathTraversal))
	assert.False(t, errors.Is(wrapped, ErrReadonlyMode))
}

func TestDropperError_As_Extraction(t *testing.T) {
	original := NewReadonlyError()
	wrapped := fmt.Errorf("outer: %w", original)

	var de *DropperError
	require.True(t, errors.As(wrapped, &de))
	assert.Equal(t, http.StatusForbidden, de.StatusCode)
	assert.Equal(t, ErrCodeReadonly, de.Code)
	assert.Equal(t, ErrMsgReadonlyMode, de.SafeMsg)
}

func TestDropperError_Unwrap_MultiError(t *testing.T) {
	inner := fmt.Errorf("os error")
	de := NewWriteFileError(inner)

	errs := de.Unwrap()
	assert.Len(t, errs, 2, "should return both sentinel and wrapped")
	assert.Equal(t, ErrWriteFile, errs[0])
	assert.Equal(t, inner, errs[1])

	// Sentinel-only error (no wrapped) returns single-element slice.
	de2 := NewPathTraversalError()
	errs2 := de2.Unwrap()
	assert.Len(t, errs2, 1)
	assert.Equal(t, ErrPathTraversal, errs2[0])
}

func TestDropperError_Unwrap_CausalChain(t *testing.T) {
	// errors.Is should match both sentinel AND the wrapped causal error.
	inner := fmt.Errorf("wrapped: %w", os.ErrPermission)
	de := NewWriteFileError(inner)

	assert.True(t, errors.Is(de, ErrWriteFile), "should match sentinel")
	assert.True(t, errors.Is(de, os.ErrPermission), "should match wrapped causal error")
	assert.False(t, errors.Is(de, ErrPathTraversal), "should not match unrelated sentinel")
}

func TestMapDropperError_AllSentinels(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantMsg    string
	}{
		{"path too long", NewPathTooLongError(), http.StatusBadRequest, ErrCodePathTooLong, ErrMsgPathTooLong},
		{"path traversal", NewPathTraversalError(), http.StatusForbidden, ErrCodeForbidden, ErrMsgPathTraversal},
		{"path resolution", NewPathResolutionError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgPathResolution},
		{"readonly", NewReadonlyError(), http.StatusForbidden, ErrCodeReadonly, ErrMsgReadonlyMode},
		{"ext not allowed", NewExtNotAllowedError(), http.StatusBadRequest, ErrCodeExtNotAllowed, ErrMsgExtNotAllowed},
		{"not directory", NewNotDirectoryError(), http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNotDirectory},
		{"not file", NewNotFileError(), http.StatusBadRequest, ErrCodeBadRequest, ErrMsgNotFile},
		{"file too large", NewFileTooLargeError(), http.StatusRequestEntityTooLarge, ErrCodeFileTooLarge, ErrMsgFileTooLarge},
		{"file exists", NewFileExistsError(), http.StatusConflict, ErrCodeInternal, ErrMsgFileExists},
		{"list dir", NewListDirError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgListDir},
		{"create dir", NewCreateDirError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgCreateDir},
		{"write file", NewWriteFileError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgWriteFile},
		{"temp file", NewTempFileError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgTempFile},
		{"rename file", NewRenameFileError(nil), http.StatusInternalServerError, ErrCodeInternal, ErrMsgRenameFile},
		{"file stat", NewFileStatError(nil), http.StatusNotFound, ErrCodeNotFound, ErrMsgFileStat},
		{"invalid relpath", NewInvalidRelPathError(), http.StatusBadRequest, ErrCodeInvalidRelPath, ErrMsgInvalidRelPath},
		{"csrf rejected", NewCSRFError(), http.StatusForbidden, ErrCodeCSRF, ErrMsgCSRFOriginMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code, msg := MapDropperError(tt.err)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantCode, code)
			assert.Equal(t, tt.wantMsg, msg)
		})
	}
}

func TestMapDropperError_UnknownError(t *testing.T) {
	err := fmt.Errorf("some random error")
	status, code, msg := MapDropperError(err)

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, ErrCodeInternal, code)
	assert.Equal(t, ErrMsgInternal, msg)
}

func TestMapDropperError_WrappedDropperError(t *testing.T) {
	// MapDropperError should find DropperError through wrapping.
	inner := NewExtNotAllowedError()
	wrapped := fmt.Errorf("handler: %w", inner)

	status, code, msg := MapDropperError(wrapped)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Equal(t, ErrCodeExtNotAllowed, code)
	assert.Equal(t, ErrMsgExtNotAllowed, msg)
}

func TestSafeErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"dropper error returns safe msg", NewPathTraversalError(), ErrMsgPathTraversal},
		{"unknown error returns generic", fmt.Errorf("internal path /tmp/x"), ErrMsgInternal},
		{"wrapped dropper error", fmt.Errorf("ctx: %w", NewReadonlyError()), ErrMsgReadonlyMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, SafeErrorMessage(tt.err))
		})
	}
}

func TestDropperError_NoPathLeakage(t *testing.T) {
	// Wrapped OS errors contain paths — SafeMsg must NOT.
	inner := fmt.Errorf("open /tmp/secret/data.bin: permission denied")
	de := NewWriteFileError(inner)

	assert.NotContains(t, de.SafeMsg, "/tmp")
	assert.NotContains(t, de.SafeMsg, "secret")
	assert.Equal(t, ErrMsgWriteFile, de.SafeMsg)

	// SafeErrorMessage also must not leak.
	assert.Equal(t, ErrMsgWriteFile, SafeErrorMessage(de))
}
