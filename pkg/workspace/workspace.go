package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultWorkspaceDirName = ".miniclaw/workspace"

// Guard resolves and validates tool paths against a workspace root.
type Guard struct {
	rootPath            string
	restrictToWorkspace bool
}

// NewGuard resolves a workspace path and ensures the directory exists.
func NewGuard(workspacePath string) (*Guard, error) {
	return NewGuardWithPolicy(workspacePath, true)
}

// NewGuardWithPolicy resolves a workspace path and applies containment policy.
func NewGuardWithPolicy(workspacePath string, restrictToWorkspace bool) (*Guard, error) {
	resolved, err := ResolveRoot(workspacePath)
	if err != nil {
		return nil, err
	}

	return &Guard{rootPath: resolved, restrictToWorkspace: restrictToWorkspace}, nil
}

// ResolveRoot normalizes workspace path input and creates it when missing.
func ResolveRoot(workspacePath string) (string, error) {
	trimmed := strings.TrimSpace(workspacePath)
	if trimmed == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		trimmed = filepath.Join(homeDir, defaultWorkspaceDirName)
	}

	expanded, err := expandHome(trimmed)
	if err != nil {
		return "", err
	}

	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve absolute workspace path: %w", err)
	}

	cleanPath := filepath.Clean(absPath)
	if err := os.MkdirAll(cleanPath, 0o755); err != nil {
		return "", fmt.Errorf("create workspace directory: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", NormalizeIOError(err, "resolve workspace root")
	}

	return filepath.Clean(resolved), nil
}

// Root returns the normalized absolute workspace root path.
func (g *Guard) Root() string {
	if g == nil {
		return ""
	}

	return g.rootPath
}

// ResolvePath validates and returns a canonical absolute path inside the workspace.
func (g *Guard) ResolvePath(inputPath string) (string, error) {
	if g == nil {
		return "", NewError(ErrorIO, "workspace guard is nil")
	}

	trimmed := strings.TrimSpace(inputPath)
	if trimmed == "" {
		return "", NewError(ErrorInvalidPath, "path must not be empty")
	}

	candidate := trimmed
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(g.rootPath, candidate)
	}

	absPath, err := filepath.Abs(candidate)
	if err != nil {
		return "", NewError(ErrorInvalidPath, "path could not be resolved")
	}

	cleanPath := filepath.Clean(absPath)
	effectivePath, err := canonicalPath(cleanPath)
	if err != nil {
		return "", err
	}

	if g.shouldEnforceContainment() && !isWithin(g.rootPath, effectivePath) {
		return "", NewError(ErrorOutsideWorkspace, "resolved path escapes workspace")
	}

	return effectivePath, nil
}

// EnsureContained re-checks containment right before mutating operations.
func (g *Guard) EnsureContained(path string) error {
	effectivePath, err := canonicalPath(path)
	if err != nil {
		return err
	}

	if g.shouldEnforceContainment() && !isWithin(g.rootPath, effectivePath) {
		return NewError(ErrorOutsideWorkspace, "resolved path escapes workspace")
	}

	return nil
}

// RelPath returns a workspace-relative path when representable.
func (g *Guard) RelPath(path string) string {
	if g == nil {
		return filepath.Clean(path)
	}

	rel, err := filepath.Rel(g.rootPath, path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return filepath.Clean(path)
	}
	if rel == "." {
		return "."
	}

	return filepath.Clean(rel)
}

func canonicalPath(path string) (string, error) {
	evaluated, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(evaluated), nil
	}
	if !os.IsNotExist(err) {
		return "", NormalizeIOError(err, "resolve path")
	}

	parent, remainder, splitErr := nearestExistingParent(path)
	if splitErr != nil {
		return "", splitErr
	}

	evaluatedParent, evalErr := filepath.EvalSymlinks(parent)
	if evalErr != nil {
		return "", NormalizeIOError(evalErr, "resolve path")
	}

	return filepath.Clean(filepath.Join(evaluatedParent, remainder)), nil
}

func nearestExistingParent(path string) (string, string, error) {
	current := filepath.Clean(path)
	parts := make([]string, 0)

	for {
		if _, err := os.Lstat(current); err == nil {
			remainder := ""
			for i := len(parts) - 1; i >= 0; i-- {
				remainder = filepath.Join(remainder, parts[i])
			}
			return current, remainder, nil
		}

		base := filepath.Base(current)
		if base == "." || base == string(filepath.Separator) {
			break
		}
		parts = append(parts, base)

		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}

	return "", "", NewError(ErrorInvalidPath, "path could not be resolved")
}

func expandHome(path string) (string, error) {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return home, nil
	}

	prefix := "~" + string(filepath.Separator)
	if strings.HasPrefix(path, prefix) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, prefix)), nil
	}

	return path, nil
}

func isWithin(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." {
		return false
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}

	return !filepath.IsAbs(rel)
}

func (g *Guard) shouldEnforceContainment() bool {
	// Phase 1 keeps workspace containment mandatory even when restriction is disabled.
	_ = g.restrictToWorkspace
	return true
}
