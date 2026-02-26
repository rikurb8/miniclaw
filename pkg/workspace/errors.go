package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

const (
	ErrorInvalidPath      = "invalid_path"
	ErrorOutsideWorkspace = "outside_workspace"
	ErrorPathNotFound     = "path_not_found"
	ErrorPermissionDenied = "permission_denied"
	ErrorIO               = "io_error"
	ErrorAmbiguousEdit    = "ambiguous_edit"
	ErrorEditNotFound     = "edit_not_found"
)

// Error represents a stable, categorized workspace/tooling failure.
type Error struct {
	Category string
	Detail   string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Detail == "" {
		return e.Category
	}

	return fmt.Sprintf("%s: %s", e.Category, e.Detail)
}

// NewError creates a categorized workspace error.
func NewError(category string, detail string) error {
	return &Error{Category: category, Detail: detail}
}

// CategoryFromError returns the stable category for an error when available.
func CategoryFromError(err error) string {
	if err == nil {
		return ""
	}

	var categorized *Error
	if errors.As(err, &categorized) {
		return categorized.Category
	}

	if errors.Is(err, fs.ErrNotExist) {
		return ErrorPathNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return ErrorPermissionDenied
	}

	return ErrorIO
}

// NormalizeIOError converts OS-level errors into stable category errors.
func NormalizeIOError(err error, detail string) error {
	if err == nil {
		return nil
	}

	category := CategoryFromError(err)
	if detail == "" {
		detail = err.Error()
	}

	// Keep os.PathError context out of user-visible text by default.
	if category == ErrorPathNotFound {
		return NewError(category, "path does not exist")
	}
	if category == ErrorPermissionDenied {
		return NewError(category, "operation not permitted")
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return NewError(category, pathErr.Err.Error())
	}

	return NewError(category, detail)
}
