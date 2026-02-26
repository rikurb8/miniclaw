package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRootExpandsHomeAndCreatesDirectory(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	root, err := ResolveRoot("~/agent-workspace")
	if err != nil {
		t.Fatalf("ResolveRoot error: %v", err)
	}

	want, err := filepath.EvalSymlinks(filepath.Join(homeDir, "agent-workspace"))
	if err != nil {
		t.Fatalf("EvalSymlinks error: %v", err)
	}
	if root != want {
		t.Fatalf("ResolveRoot root = %q, want %q", root, want)
	}

	if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
		t.Fatalf("workspace directory missing: %v", statErr)
	}
}

func TestResolvePathRejectsEmpty(t *testing.T) {
	guard := mustGuard(t)

	_, err := guard.ResolvePath("  ")
	if CategoryFromError(err) != ErrorInvalidPath {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorInvalidPath)
	}
}

func TestResolvePathRelativeInsideWorkspace(t *testing.T) {
	guard := mustGuard(t)

	resolved, err := guard.ResolvePath("notes/todo.txt")
	if err != nil {
		t.Fatalf("ResolvePath error: %v", err)
	}

	if !strings.HasPrefix(resolved, guard.Root()+string(filepath.Separator)) {
		t.Fatalf("resolved path = %q is not inside root %q", resolved, guard.Root())
	}
}

func TestResolvePathRejectsTraversalEscape(t *testing.T) {
	guard := mustGuard(t)

	_, err := guard.ResolvePath("../escape.txt")
	if CategoryFromError(err) != ErrorOutsideWorkspace {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorOutsideWorkspace)
	}
}

func TestResolvePathRejectsAbsoluteOutsideWorkspace(t *testing.T) {
	guard := mustGuard(t)
	outsideDir := t.TempDir()

	_, err := guard.ResolvePath(filepath.Join(outsideDir, "external.txt"))
	if CategoryFromError(err) != ErrorOutsideWorkspace {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorOutsideWorkspace)
	}
}

func TestResolvePathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	linkPath := filepath.Join(root, "out-link")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	guard, err := NewGuard(root)
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	_, err = guard.ResolvePath("out-link/file.txt")
	if CategoryFromError(err) != ErrorOutsideWorkspace {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorOutsideWorkspace)
	}
}

func TestEnsureContainedRejectsOutsidePath(t *testing.T) {
	guard := mustGuard(t)
	outsideDir := t.TempDir()

	err := guard.EnsureContained(filepath.Join(outsideDir, "file.txt"))
	if CategoryFromError(err) != ErrorOutsideWorkspace {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorOutsideWorkspace)
	}
}

func TestNewGuardWithPolicyStillEnforcesContainmentInPhaseOne(t *testing.T) {
	guard, err := NewGuardWithPolicy(t.TempDir(), false)
	if err != nil {
		t.Fatalf("NewGuardWithPolicy error: %v", err)
	}

	outsideDir := t.TempDir()
	_, err = guard.ResolvePath(filepath.Join(outsideDir, "outside.txt"))
	if CategoryFromError(err) != ErrorOutsideWorkspace {
		t.Fatalf("error category = %q, want %q", CategoryFromError(err), ErrorOutsideWorkspace)
	}
}

func mustGuard(t *testing.T) *Guard {
	t.Helper()

	guard, err := NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	return guard
}
