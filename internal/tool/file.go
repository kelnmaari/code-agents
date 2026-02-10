package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 1 << 20 // 1 MB

// safePath resolves relPath relative to workDir and verifies it stays inside workDir.
func safePath(workDir, relPath string) (string, error) {
	abs := filepath.Join(workDir, filepath.Clean(relPath))
	absClean := filepath.Clean(abs)
	workClean := filepath.Clean(workDir)
	if !strings.HasPrefix(absClean, workClean+string(filepath.Separator)) && absClean != workClean {
		return "", fmt.Errorf("path traversal detected: %s", relPath)
	}
	return absClean, nil
}

// --- ReadFileTool ---

type ReadFileTool struct {
	workDir string
}

func NewReadFile(workDir string) *ReadFileTool {
	return &ReadFileTool{workDir: workDir}
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string  { return "Read the contents of a file at the given relative path" }
func (t *ReadFileTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path to the file",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	absPath, err := safePath(t.workDir, params.Path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}
	if info.Size() > maxFileSize {
		return fmt.Sprintf("Error: file too large (%d bytes, max %d)", info.Size(), maxFileSize), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	return string(data), nil
}

// --- WriteFileTool ---

type WriteFileTool struct {
	workDir string
}

func NewWriteFile(workDir string) *WriteFileTool {
	return &WriteFileTool{workDir: workDir}
}

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string  { return "Write content to a file, creating directories as needed" }
func (t *WriteFileTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path to the file",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "File content to write",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	absPath, err := safePath(t.workDir, params.Path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Sprintf("Error creating directory: %s", err), nil
	}

	if err := os.WriteFile(absPath, []byte(params.Content), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}

	return fmt.Sprintf("File written: %s (%d bytes)", params.Path, len(params.Content)), nil
}

// --- ListDirTool ---

type ListDirTool struct {
	workDir string
}

func NewListDir(workDir string) *ListDirTool {
	return &ListDirTool{workDir: workDir}
}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string  { return "List files and directories at the given relative path" }
func (t *ListDirTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path to directory. Use '.' for root.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}
	if params.Path == "" {
		params.Path = "."
	}

	absPath, err := safePath(t.workDir, params.Path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	entries, err := os.ReadDir(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading directory: %s", err), nil
	}

	var sb strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		sb.WriteString(name)
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
