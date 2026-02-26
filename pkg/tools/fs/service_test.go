package fs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"miniclaw/pkg/workspace"
)

func TestReadWriteAppendListEditHappyPaths(t *testing.T) {
	service, guard := mustService(t)
	ctx := context.Background()

	writeResult, err := service.WriteFile(ctx, "notes/file.txt", "hello")
	if err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if writeResult.BytesWritten != 5 {
		t.Fatalf("BytesWritten = %d, want 5", writeResult.BytesWritten)
	}

	appendResult, err := service.AppendFile(ctx, "notes/file.txt", " world")
	if err != nil {
		t.Fatalf("AppendFile error: %v", err)
	}
	if appendResult.BytesAppended != 6 {
		t.Fatalf("BytesAppended = %d, want 6", appendResult.BytesAppended)
	}

	readResult, err := service.ReadFile(ctx, "notes/file.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if readResult.Content != "hello world" {
		t.Fatalf("ReadFile content = %q, want %q", readResult.Content, "hello world")
	}

	editResult, err := service.EditFile(ctx, "notes/file.txt", "world", "MiniClaw", false)
	if err != nil {
		t.Fatalf("EditFile error: %v", err)
	}
	if editResult.ReplacedCount != 1 {
		t.Fatalf("ReplacedCount = %d, want 1", editResult.ReplacedCount)
	}

	listed, err := service.ListDir(ctx, "notes")
	if err != nil {
		t.Fatalf("ListDir error: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "file.txt" {
		t.Fatalf("ListDir entries = %+v, want one file.txt", listed.Entries)
	}

	rel := guard.RelPath(readResult.Path)
	if rel != filepath.Join("notes", "file.txt") {
		t.Fatalf("RelPath = %q, want %q", rel, filepath.Join("notes", "file.txt"))
	}
}

func TestReadFileNotFound(t *testing.T) {
	service, _ := mustService(t)

	_, err := service.ReadFile(context.Background(), "missing.txt")
	if workspace.CategoryFromError(err) != workspace.ErrorPathNotFound {
		t.Fatalf("error category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorPathNotFound)
	}
}

func TestReadFileRejectsBinary(t *testing.T) {
	service, guard := mustService(t)
	path := filepath.Join(guard.Root(), "bin.dat")
	if err := os.WriteFile(path, []byte{0x00, 0x01, 0x02}, 0o600); err != nil {
		t.Fatalf("write binary file: %v", err)
	}

	_, err := service.ReadFile(context.Background(), "bin.dat")
	if workspace.CategoryFromError(err) != workspace.ErrorIO {
		t.Fatalf("error category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorIO)
	}
}

func TestEditFileErrors(t *testing.T) {
	service, _ := mustService(t)
	ctx := context.Background()

	if _, err := service.WriteFile(ctx, "edit.txt", "a b a"); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	_, err := service.EditFile(ctx, "edit.txt", "zzz", "x", false)
	if workspace.CategoryFromError(err) != workspace.ErrorEditNotFound {
		t.Fatalf("error category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorEditNotFound)
	}

	_, err = service.EditFile(ctx, "edit.txt", "a", "x", false)
	if workspace.CategoryFromError(err) != workspace.ErrorAmbiguousEdit {
		t.Fatalf("error category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorAmbiguousEdit)
	}

	result, err := service.EditFile(ctx, "edit.txt", "a", "x", true)
	if err != nil {
		t.Fatalf("EditFile replace_all error: %v", err)
	}
	if result.ReplacedCount != 2 {
		t.Fatalf("ReplacedCount = %d, want 2", result.ReplacedCount)
	}
}

func TestWriteAndAppendEnforceSizeLimit(t *testing.T) {
	service, _ := mustService(t)
	service.maxWriteBytes = 8

	large := strings.Repeat("x", 9)
	if _, err := service.WriteFile(context.Background(), "too-big.txt", large); workspace.CategoryFromError(err) != workspace.ErrorIO {
		t.Fatalf("WriteFile category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorIO)
	}

	if _, err := service.AppendFile(context.Background(), "too-big.txt", large); workspace.CategoryFromError(err) != workspace.ErrorIO {
		t.Fatalf("AppendFile category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorIO)
	}
}

func TestListDirTruncatesDeterministically(t *testing.T) {
	service, guard := mustService(t)
	service.maxListEntries = 2

	for _, name := range []string{"b.txt", "a.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(guard.Root(), name), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed file %s: %v", name, err)
		}
	}

	result, err := service.ListDir(context.Background(), ".")
	if err != nil {
		t.Fatalf("ListDir error: %v", err)
	}
	if !result.Truncated {
		t.Fatal("expected truncated list")
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(result.Entries))
	}
	if result.Entries[0].Name != "a.txt" || result.Entries[1].Name != "b.txt" {
		t.Fatalf("entries order = %q, %q, want a.txt, b.txt", result.Entries[0].Name, result.Entries[1].Name)
	}
}

func TestServiceRespectsCancelledContext(t *testing.T) {
	service, _ := mustService(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.WriteFile(ctx, "cancelled.txt", "hello")
	if workspace.CategoryFromError(err) != workspace.ErrorIO {
		t.Fatalf("error category = %q, want %q", workspace.CategoryFromError(err), workspace.ErrorIO)
	}
}

func mustService(t *testing.T) (*Service, *workspace.Guard) {
	t.Helper()

	guard, err := workspace.NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	return NewService(guard), guard
}
