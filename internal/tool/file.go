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
	// Resolve workDir to absolute path first
	absWorkDir, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve workdir: %w", err)
	}

	// Clean the relative path and strip any drive letter / leading slash to force relative
	cleaned := filepath.Clean(relPath)
	if filepath.IsAbs(cleaned) {
		// If the LLM sends an absolute path, try to make it relative to workDir
		rel, err := filepath.Rel(absWorkDir, cleaned)
		if err != nil {
			return "", fmt.Errorf("path traversal detected: %s", relPath)
		}
		cleaned = rel
	}

	abs := filepath.Join(absWorkDir, cleaned)
	absClean := filepath.Clean(abs)
	if !strings.HasPrefix(absClean, absWorkDir+string(filepath.Separator)) && absClean != absWorkDir {
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

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the contents of a file at the given relative path"
}
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
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file %q not found", params.Path), nil
		}
		return fmt.Sprintf("Error: %s", err), nil
	}
	if info.Size() > maxFileSize {
		return fmt.Sprintf("Error: file too large (%d bytes, max %d)", info.Size(), maxFileSize), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	// Add line numbers for use with replace_lines tool
	lines := strings.Split(string(data), "\n")
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%4d: %s\n", i+1, line))
	}

	return sb.String(), nil
}

// --- WriteFileTool ---

type WriteFileTool struct {
	workDir string
}

func NewWriteFile(workDir string) *WriteFileTool {
	return &WriteFileTool{workDir: workDir}
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Create a NEW file or fully overwrite an existing file. WARNING: this replaces the ENTIRE file. To modify specific parts of an existing file, use edit_file instead."
}
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

func (t *ListDirTool) Name() string { return "list_dir" }
func (t *ListDirTool) Description() string {
	return "List files and directories at the given relative path"
}
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

// --- EditFileTool ---

type EditFileTool struct {
	workDir string
}

func NewEditFile(workDir string) *EditFileTool {
	return &EditFileTool{workDir: workDir}
}

func (t *EditFileTool) Name() string { return "edit_file" }
func (t *EditFileTool) Description() string {
	return "Edit an existing file by searching for an exact text match and replacing it. Use this instead of write_file when modifying existing files."
}
func (t *EditFileTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path to the file to edit",
			},
			"search": map[string]interface{}{
				"type":        "string",
				"description": "The exact text to find in the file. Must match exactly, including whitespace and newlines.",
			},
			"replace": map[string]interface{}{
				"type":        "string",
				"description": "The text to replace the search match with. Can be empty to delete the matched text.",
			},
		},
		"required": []string{"path", "search", "replace"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Search  string `json:"search"`
		Replace string `json:"replace"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	if params.Search == "" {
		return "Error: search text cannot be empty", nil
	}

	absPath, err := safePath(t.workDir, params.Path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	content := string(data)

	// Count occurrences
	count := strings.Count(content, params.Search)
	if count == 0 {
		return fmt.Sprintf("Error: search text not found in %s. Make sure the text matches exactly, including whitespace.", params.Path), nil
	}
	if count > 1 {
		return fmt.Sprintf("Error: search text found %d times in %s. Please use a more specific search string that matches exactly once.", count, params.Path), nil
	}

	// Replace
	newContent := strings.Replace(content, params.Search, params.Replace, 1)

	if err := os.WriteFile(absPath, []byte(newContent), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}

	return fmt.Sprintf("File edited: %s (replaced %d bytes with %d bytes)", params.Path, len(params.Search), len(params.Replace)), nil
}

// --- ReplaceLinesTool ---

type ReplaceLinesTool struct {
	workDir string
}

func NewReplaceLines(workDir string) *ReplaceLinesTool {
	return &ReplaceLinesTool{workDir: workDir}
}

func (t *ReplaceLinesTool) Name() string { return "replace_lines" }
func (t *ReplaceLinesTool) Description() string {
	return "Replace a range of lines in a file with new content. Safer than edit_file — uses line numbers instead of text matching. Always read_file first to see line numbers."
}
func (t *ReplaceLinesTool) Parameters() interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Relative path to the file to edit",
			},
			"start_line": map[string]interface{}{
				"type":        "integer",
				"description": "First line number to replace (1-based, inclusive)",
			},
			"end_line": map[string]interface{}{
				"type":        "integer",
				"description": "Last line number to replace (1-based, inclusive). Use same as start_line to replace a single line.",
			},
			"new_content": map[string]interface{}{
				"type":        "string",
				"description": "New content to insert in place of the specified line range. Can be multiple lines (use \\n). Use empty string to delete lines.",
			},
		},
		"required": []string{"path", "start_line", "end_line", "new_content"},
	}
}

func (t *ReplaceLinesTool) Execute(_ context.Context, args string) (string, error) {
	var params struct {
		Path       string `json:"path"`
		StartLine  int    `json:"start_line"`
		EndLine    int    `json:"end_line"`
		NewContent string `json:"new_content"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return fmt.Sprintf("Error parsing arguments: %s", err), nil
	}

	if params.StartLine < 1 {
		return "Error: start_line must be >= 1", nil
	}
	if params.EndLine < params.StartLine {
		return "Error: end_line must be >= start_line", nil
	}

	absPath, err := safePath(t.workDir, params.Path)
	if err != nil {
		return fmt.Sprintf("Error: %s", err), nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %s", err), nil
	}

	lines := strings.Split(string(data), "\n")

	if params.StartLine > len(lines) {
		return fmt.Sprintf("Error: start_line %d exceeds file length (%d lines)", params.StartLine, len(lines)), nil
	}
	if params.EndLine > len(lines) {
		params.EndLine = len(lines)
	}

	// Build new file: lines before + new content + lines after
	var result []string
	result = append(result, lines[:params.StartLine-1]...)
	if params.NewContent != "" {
		newLines := strings.Split(params.NewContent, "\n")
		result = append(result, newLines...)
	}
	result = append(result, lines[params.EndLine:]...)

	newData := strings.Join(result, "\n")
	if err := os.WriteFile(absPath, []byte(newData), 0o644); err != nil {
		return fmt.Sprintf("Error writing file: %s", err), nil
	}

	replacedCount := params.EndLine - params.StartLine + 1
	return fmt.Sprintf("Replaced lines %d-%d in %s (%d lines removed, new content inserted)",
		params.StartLine, params.EndLine, params.Path, replacedCount), nil
}
