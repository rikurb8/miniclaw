package fantasy

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	core "charm.land/fantasy"

	fstools "miniclaw/pkg/tools/fs"
	"miniclaw/pkg/workspace"
)

type readFileInput struct {
	Path string `json:"path" description:"File path relative to the workspace root."`
}

type writeFileInput struct {
	Path    string `json:"path" description:"File path relative to the workspace root."`
	Content string `json:"content" description:"Full file content to write."`
}

type appendFileInput struct {
	Path    string `json:"path" description:"File path relative to the workspace root."`
	Content string `json:"content" description:"Text to append at the end of the file."`
}

type listDirInput struct {
	Path string `json:"path,omitempty" description:"Directory path relative to the workspace root. Defaults to '.' when omitted."`
}

type editFileInput struct {
	Path       string `json:"path" description:"File path relative to the workspace root."`
	OldText    string `json:"old_text" description:"Exact text to replace."`
	NewText    string `json:"new_text" description:"Replacement text."`
	ReplaceAll bool   `json:"replace_all,omitempty" description:"Replace all matches when true. Default false requires exactly one match."`
}

// BuildFSTools constructs the phase-1 filesystem tools for fantasy-agent.
func BuildFSTools(service *fstools.Service, guard *workspace.Guard) []core.AgentTool {
	if service == nil || guard == nil {
		return nil
	}

	tools := []core.AgentTool{
		core.NewAgentTool("read_file", "Read a UTF-8 text file from the workspace.", func(ctx context.Context, input readFileInput, _ core.ToolCall) (core.ToolResponse, error) {
			start := time.Now()
			result, err := service.ReadFile(ctx, input.Path)
			if err != nil {
				logToolResult("read_file", input.Path, false, time.Since(start), workspace.CategoryFromError(err))
				return toolErrorResponse(err), nil
			}

			relPath := safeRelPath(guard, result.Path)
			logToolResult("read_file", relPath, true, time.Since(start), "")
			return core.NewTextResponse(fmt.Sprintf("ok: read %d bytes from %s\n%s", result.Bytes, relPath, result.Content)), nil
		}),
		core.NewAgentTool("write_file", "Write a full text file inside the workspace.", func(ctx context.Context, input writeFileInput, _ core.ToolCall) (core.ToolResponse, error) {
			start := time.Now()
			result, err := service.WriteFile(ctx, input.Path, input.Content)
			if err != nil {
				logToolResult("write_file", input.Path, false, time.Since(start), workspace.CategoryFromError(err))
				return toolErrorResponse(err), nil
			}

			relPath := safeRelPath(guard, result.Path)
			logToolResult("write_file", relPath, true, time.Since(start), "")
			return core.NewTextResponse(fmt.Sprintf("ok: wrote %d bytes to %s", result.BytesWritten, relPath)), nil
		}),
		core.NewAgentTool("append_file", "Append text to a file inside the workspace.", func(ctx context.Context, input appendFileInput, _ core.ToolCall) (core.ToolResponse, error) {
			start := time.Now()
			result, err := service.AppendFile(ctx, input.Path, input.Content)
			if err != nil {
				logToolResult("append_file", input.Path, false, time.Since(start), workspace.CategoryFromError(err))
				return toolErrorResponse(err), nil
			}

			relPath := safeRelPath(guard, result.Path)
			logToolResult("append_file", relPath, true, time.Since(start), "")
			return core.NewTextResponse(fmt.Sprintf("ok: appended %d bytes to %s (size=%d)", result.BytesAppended, relPath, result.Size)), nil
		}),
		core.NewAgentTool("list_dir", "List directory entries inside the workspace.", func(ctx context.Context, input listDirInput, _ core.ToolCall) (core.ToolResponse, error) {
			start := time.Now()
			result, err := service.ListDir(ctx, input.Path)
			if err != nil {
				logToolResult("list_dir", input.Path, false, time.Since(start), workspace.CategoryFromError(err))
				return toolErrorResponse(err), nil
			}

			var b strings.Builder
			relPath := safeRelPath(guard, result.Path)
			fmt.Fprintf(&b, "ok: listed %d entries in %s", len(result.Entries), relPath)
			if result.Truncated {
				fmt.Fprintf(&b, " (truncated from %d)", result.Total)
			}
			for _, entry := range result.Entries {
				fmt.Fprintf(&b, "\n- %s\t%s\t%d", entry.Name, entry.Type, entry.Size)
			}
			logToolResult("list_dir", relPath, true, time.Since(start), "")

			return core.NewTextResponse(b.String()), nil
		}),
		core.NewAgentTool("edit_file", "Replace exact text in a file inside the workspace.", func(ctx context.Context, input editFileInput, _ core.ToolCall) (core.ToolResponse, error) {
			start := time.Now()
			result, err := service.EditFile(ctx, input.Path, input.OldText, input.NewText, input.ReplaceAll)
			if err != nil {
				logToolResult("edit_file", input.Path, false, time.Since(start), workspace.CategoryFromError(err))
				return toolErrorResponse(err), nil
			}

			relPath := safeRelPath(guard, result.Path)
			logToolResult("edit_file", relPath, true, time.Since(start), "")
			return core.NewTextResponse(fmt.Sprintf("ok: replaced %d match(es) in %s", result.ReplacedCount, relPath)), nil
		}),
	}

	return tools
}

func toolErrorResponse(err error) core.ToolResponse {
	if err == nil {
		return core.NewTextErrorResponse(workspace.ErrorIO + ": unknown error")
	}

	category := workspace.CategoryFromError(err)
	if category == "" {
		category = workspace.ErrorIO
	}

	message := err.Error()
	if !strings.Contains(message, category+":") && !strings.HasPrefix(message, category) {
		message = category + ": " + message
	}

	return core.NewTextErrorResponse(message)
}

func safeRelPath(guard *workspace.Guard, path string) string {
	if guard == nil {
		return filepath.Clean(path)
	}

	return guard.RelPath(path)
}

func logToolResult(toolName string, targetPath string, success bool, duration time.Duration, errorCategory string) {
	attrs := []any{
		"component", "provider.fantasy",
		"tool", toolName,
		"path", filepath.Clean(strings.TrimSpace(targetPath)),
		"success", success,
		"duration_ms", duration.Milliseconds(),
	}
	if errorCategory != "" {
		attrs = append(attrs, "error_category", errorCategory)
	}

	slog.Default().Info("Fantasy tool execution", attrs...)
}
