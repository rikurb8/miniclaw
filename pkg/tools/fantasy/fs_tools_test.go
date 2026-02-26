package fantasy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	core "charm.land/fantasy"

	fstools "miniclaw/pkg/tools/fs"
	"miniclaw/pkg/workspace"
)

func TestBuildFSToolsRegistersExpectedNames(t *testing.T) {
	guard, err := workspace.NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	tools := BuildFSTools(fstools.NewService(guard), guard)
	if len(tools) != 5 {
		t.Fatalf("tool count = %d, want 5", len(tools))
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Info().Name)
	}

	want := []string{"read_file", "write_file", "append_file", "list_dir", "edit_file"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tool[%d] name = %q, want %q", i, names[i], want[i])
		}
		if tools[i].Info().Parallel {
			t.Fatalf("tool %q unexpectedly marked parallel", tools[i].Info().Name)
		}
	}
}

func TestBuildFSToolsSchemaHasRequiredPath(t *testing.T) {
	guard, err := workspace.NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	tools := BuildFSTools(fstools.NewService(guard), guard)
	writeTool := mustTool(t, tools, "write_file")
	required := writeTool.Info().Required
	if len(required) == 0 {
		t.Fatal("expected required fields in schema")
	}
	hasPath := false
	for _, field := range required {
		if field == "path" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Fatalf("required fields = %v, expected path", required)
	}
}

func TestRecoverableToolErrorsUseTextErrorResponse(t *testing.T) {
	guard, err := workspace.NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	tools := BuildFSTools(fstools.NewService(guard), guard)
	readTool := mustTool(t, tools, "read_file")

	input, _ := json.Marshal(readFileInput{Path: "missing.txt"})
	response, runErr := readTool.Run(context.Background(), core.ToolCall{Input: string(input)})
	if runErr != nil {
		t.Fatalf("tool run should not fail fatally: %v", runErr)
	}
	if !response.IsError {
		t.Fatal("expected IsError=true for recoverable failure")
	}
	if !strings.Contains(response.Content, workspace.ErrorPathNotFound) {
		t.Fatalf("response content = %q, missing category", response.Content)
	}
}

func TestWriteAndReadToolResponses(t *testing.T) {
	guard, err := workspace.NewGuard(t.TempDir())
	if err != nil {
		t.Fatalf("NewGuard error: %v", err)
	}

	tools := BuildFSTools(fstools.NewService(guard), guard)
	writeTool := mustTool(t, tools, "write_file")
	readTool := mustTool(t, tools, "read_file")

	writeInput, _ := json.Marshal(writeFileInput{Path: "demo.txt", Content: "hello"})
	writeResponse, writeErr := writeTool.Run(context.Background(), core.ToolCall{Input: string(writeInput)})
	if writeErr != nil {
		t.Fatalf("write tool error: %v", writeErr)
	}
	if writeResponse.IsError {
		t.Fatalf("write response unexpectedly marked error: %q", writeResponse.Content)
	}

	readInput, _ := json.Marshal(readFileInput{Path: "demo.txt"})
	readResponse, readErr := readTool.Run(context.Background(), core.ToolCall{Input: string(readInput)})
	if readErr != nil {
		t.Fatalf("read tool error: %v", readErr)
	}
	if readResponse.IsError {
		t.Fatalf("read response unexpectedly marked error: %q", readResponse.Content)
	}
	if !strings.Contains(readResponse.Content, "hello") {
		t.Fatalf("read response = %q, expected content", readResponse.Content)
	}
}

func mustTool(t *testing.T, tools []core.AgentTool, name string) core.AgentTool {
	t.Helper()

	for _, tool := range tools {
		if tool.Info().Name == name {
			return tool
		}
	}

	t.Fatalf("tool %q not found", name)
	return nil
}
