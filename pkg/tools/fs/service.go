package fs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"miniclaw/pkg/workspace"
)

const (
	MaxReadBytes             = 256 * 1024
	MaxWriteBytes            = 1024 * 1024
	MaxListEntries           = 500
	MaxToolOperationDuration = 10 * time.Second
)

// Service executes bounded filesystem operations inside a workspace.
type Service struct {
	guard                    *workspace.Guard
	maxReadBytes             int
	maxWriteBytes            int
	maxListEntries           int
	maxToolOperationDuration time.Duration
}

type ReadResult struct {
	Path    string
	Content string
	Bytes   int
}

type WriteResult struct {
	Path         string
	BytesWritten int
}

type AppendResult struct {
	Path          string
	BytesAppended int
	Size          int64
}

type ListEntry struct {
	Name  string
	Type  string
	Size  int64
	IsDir bool
}

type ListResult struct {
	Path      string
	Entries   []ListEntry
	Truncated bool
	Total     int
}

type EditResult struct {
	Path          string
	Matches       int
	ReplacedCount int
	BytesWritten  int
}

// NewService creates a workspace-bounded filesystem service.
func NewService(guard *workspace.Guard) *Service {
	return &Service{
		guard:                    guard,
		maxReadBytes:             MaxReadBytes,
		maxWriteBytes:            MaxWriteBytes,
		maxListEntries:           MaxListEntries,
		maxToolOperationDuration: MaxToolOperationDuration,
	}
}

func (s *Service) ReadFile(ctx context.Context, path string) (ReadResult, error) {
	ctx, cancel := s.withOperationContext(ctx)
	defer cancel()

	resolvedPath, err := s.guard.ResolvePath(path)
	if err != nil {
		return ReadResult{}, err
	}

	if err := checkContext(ctx); err != nil {
		return ReadResult{}, err
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ReadResult{}, workspace.NormalizeIOError(err, "read failed")
	}

	if len(content) > s.maxReadBytes {
		return ReadResult{}, workspace.NewError(workspace.ErrorIO, fmt.Sprintf("file exceeds max_read_bytes (%d)", s.maxReadBytes))
	}
	if err := ensureText(content); err != nil {
		return ReadResult{}, err
	}

	return ReadResult{
		Path:    resolvedPath,
		Content: string(content),
		Bytes:   len(content),
	}, nil
}

func (s *Service) WriteFile(ctx context.Context, path string, content string) (WriteResult, error) {
	ctx, cancel := s.withOperationContext(ctx)
	defer cancel()

	if len(content) > s.maxWriteBytes {
		return WriteResult{}, workspace.NewError(workspace.ErrorIO, fmt.Sprintf("content exceeds max_write_bytes (%d)", s.maxWriteBytes))
	}
	if err := checkContext(ctx); err != nil {
		return WriteResult{}, err
	}

	resolvedPath, err := s.guard.ResolvePath(path)
	if err != nil {
		return WriteResult{}, err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(resolvedPath); statErr == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return WriteResult{}, workspace.NormalizeIOError(statErr, "stat failed")
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return WriteResult{}, workspace.NormalizeIOError(err, "create parent directory failed")
	}

	if err := s.guard.EnsureContained(resolvedPath); err != nil {
		return WriteResult{}, err
	}

	if err := atomicWrite(resolvedPath, []byte(content), mode); err != nil {
		return WriteResult{}, workspace.NormalizeIOError(err, "write failed")
	}

	return WriteResult{Path: resolvedPath, BytesWritten: len(content)}, nil
}

func (s *Service) AppendFile(ctx context.Context, path string, content string) (AppendResult, error) {
	ctx, cancel := s.withOperationContext(ctx)
	defer cancel()

	if len(content) > s.maxWriteBytes {
		return AppendResult{}, workspace.NewError(workspace.ErrorIO, fmt.Sprintf("content exceeds max_write_bytes (%d)", s.maxWriteBytes))
	}
	if err := checkContext(ctx); err != nil {
		return AppendResult{}, err
	}

	resolvedPath, err := s.guard.ResolvePath(path)
	if err != nil {
		return AppendResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(resolvedPath), 0o755); err != nil {
		return AppendResult{}, workspace.NormalizeIOError(err, "create parent directory failed")
	}

	if err := s.guard.EnsureContained(resolvedPath); err != nil {
		return AppendResult{}, err
	}

	file, err := os.OpenFile(resolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return AppendResult{}, workspace.NormalizeIOError(err, "open append target failed")
	}
	defer file.Close()

	bytesWritten, err := file.WriteString(content)
	if err != nil {
		return AppendResult{}, workspace.NormalizeIOError(err, "append failed")
	}

	info, err := file.Stat()
	if err != nil {
		return AppendResult{}, workspace.NormalizeIOError(err, "stat append target failed")
	}

	return AppendResult{Path: resolvedPath, BytesAppended: bytesWritten, Size: info.Size()}, nil
}

func (s *Service) ListDir(ctx context.Context, path string) (ListResult, error) {
	ctx, cancel := s.withOperationContext(ctx)
	defer cancel()

	if strings.TrimSpace(path) == "" {
		path = "."
	}
	if err := checkContext(ctx); err != nil {
		return ListResult{}, err
	}

	resolvedPath, err := s.guard.ResolvePath(path)
	if err != nil {
		return ListResult{}, err
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return ListResult{}, workspace.NormalizeIOError(err, "list directory failed")
	}

	sort.Slice(entries, func(i int, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	limited := entries
	truncated := false
	if len(entries) > s.maxListEntries {
		limited = entries[:s.maxListEntries]
		truncated = true
	}

	resultEntries := make([]ListEntry, 0, len(limited))
	for _, entry := range limited {
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return ListResult{}, workspace.NormalizeIOError(infoErr, "read directory metadata failed")
		}

		entryType := "file"
		if entry.IsDir() {
			entryType = "dir"
		}

		resultEntries = append(resultEntries, ListEntry{
			Name:  entry.Name(),
			Type:  entryType,
			Size:  entryInfo.Size(),
			IsDir: entry.IsDir(),
		})
	}

	return ListResult{
		Path:      resolvedPath,
		Entries:   resultEntries,
		Truncated: truncated,
		Total:     len(entries),
	}, nil
}

func (s *Service) EditFile(ctx context.Context, path string, oldText string, newText string, replaceAll bool) (EditResult, error) {
	ctx, cancel := s.withOperationContext(ctx)
	defer cancel()

	if oldText == "" {
		return EditResult{}, workspace.NewError(workspace.ErrorInvalidPath, "old_text must not be empty")
	}
	if err := checkContext(ctx); err != nil {
		return EditResult{}, err
	}

	resolvedPath, err := s.guard.ResolvePath(path)
	if err != nil {
		return EditResult{}, err
	}

	raw, err := os.ReadFile(resolvedPath)
	if err != nil {
		return EditResult{}, workspace.NormalizeIOError(err, "read failed")
	}
	if err := ensureText(raw); err != nil {
		return EditResult{}, err
	}

	original := string(raw)
	matches := strings.Count(original, oldText)
	if matches == 0 {
		return EditResult{}, workspace.NewError(workspace.ErrorEditNotFound, "old_text not found")
	}
	if matches > 1 && !replaceAll {
		return EditResult{}, workspace.NewError(workspace.ErrorAmbiguousEdit, "old_text matched multiple locations")
	}

	updated := original
	replaced := 1
	if replaceAll {
		replaced = matches
		updated = strings.ReplaceAll(original, oldText, newText)
	} else {
		updated = strings.Replace(original, oldText, newText, 1)
	}

	if len(updated) > s.maxWriteBytes {
		return EditResult{}, workspace.NewError(workspace.ErrorIO, fmt.Sprintf("content exceeds max_write_bytes (%d)", s.maxWriteBytes))
	}

	if err := s.guard.EnsureContained(resolvedPath); err != nil {
		return EditResult{}, err
	}

	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(resolvedPath); statErr == nil {
		mode = info.Mode().Perm()
	}

	if err := atomicWrite(resolvedPath, []byte(updated), mode); err != nil {
		return EditResult{}, workspace.NormalizeIOError(err, "write failed")
	}

	return EditResult{
		Path:          resolvedPath,
		Matches:       matches,
		ReplacedCount: replaced,
		BytesWritten:  len(updated),
	}, nil
}

func (s *Service) withOperationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}

	if s.maxToolOperationDuration <= 0 {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, s.maxToolOperationDuration)
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return workspace.NewError(workspace.ErrorIO, err.Error())
	}

	return nil
}

func ensureText(content []byte) error {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return workspace.NewError(workspace.ErrorIO, "file appears to be binary or invalid utf-8")
	}

	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	parentDir := filepath.Dir(path)
	tmp, err := os.CreateTemp(parentDir, ".miniclaw-tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	cleanup = false
	return nil
}
