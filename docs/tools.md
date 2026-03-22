# Инструменты агентов (Tools)

## Обзор

Tools -- механизм взаимодействия агентов с внешним миром. Каждый tool реализует интерфейс `Tool` и регистрируется в `Registry`. LLM вызывает tools через OpenAI function calling API.

## Распределение tools по ролям

| Tool | Planner | Subplanner | Worker |
|------|:-------:|:----------:|:------:|
| `create_task` | + | + | - |
| `submit_handoff` | - | + | - |
| `complete_task` | - | - | + |
| `read_file` | + | - | + |
| `write_file` | - | - | + |
| `edit_file` | - | - | + |
| `replace_lines` | - | - | + |
| `list_dir` | + | - | + |
| `shell_exec` | + | - | + |
| `git_status` | - | - | + |
| `git_diff` | - | - | + |
| `git_commit` | - | - | + |

Planner получает `read_file`, `list_dir`, `shell_exec` для разведки кодовой базы перед планированием — чтобы создавать обоснованные задачи с правильным scope и constraints. Subplanner работает только с task tools (`create_task`, `submit_handoff`) без доступа к файловой системе.

---

## Task Tools (`internal/tool/task.go`)

### `create_task`

Создает новую задачу в Queue. Доступен planners и subplanners.

**Parameters JSON Schema:**
```json
{
  "type": "object",
  "properties": {
    "title": {
      "type": "string",
      "description": "Brief task title"
    },
    "description": {
      "type": "string",
      "description": "Detailed task description with context"
    },
    "scope": {
      "type": "string",
      "description": "Files or areas this task affects (e.g. 'auth/ directory')"
    },
    "constraints": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Constraints for the worker (what NOT to do, boundaries)"
    },
    "priority": {
      "type": "string",
      "enum": ["low", "normal", "high"],
      "description": "Task priority. Default: normal"
    },
    "is_subplan": {
      "type": "boolean",
      "description": "If true, task will be handled by a subplanner instead of a worker. Use for tasks too large for a single worker."
    }
  },
  "required": ["title", "description"]
}
```

**Возвращает:** `"Task created: {id} - {title}"`

**Пример вызова LLM:**
```json
{
  "name": "create_task",
  "arguments": "{\"title\": \"Add JWT middleware\", \"description\": \"Create authentication middleware using JWT tokens. Validate tokens from Authorization header, extract user claims, and pass them in request context.\", \"scope\": \"middleware/auth.go\", \"constraints\": [\"Use golang-jwt/jwt/v5\", \"Do not modify existing middleware chain\", \"Include unit tests\"], \"priority\": \"high\", \"is_subplan\": false}"
}
```

---

### `complete_task`

Завершает задачу и подает Handoff документ в Queue. Доступен только workers.

**Parameters JSON Schema:**
```json
{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "ID of the task being completed"
    },
    "summary": {
      "type": "string",
      "description": "Summary of what was done"
    },
    "findings": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Things discovered during work"
    },
    "concerns": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Potential issues or risks"
    },
    "feedback": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Suggestions for the planner"
    },
    "files_changed": {
      "type": "array",
      "items": {"type": "string"},
      "description": "List of files that were created or modified"
    }
  },
  "required": ["task_id", "summary"]
}
```

**Возвращает:** `"Task {id} completed. Handoff submitted."`

**Пример:**
```json
{
  "name": "complete_task",
  "arguments": "{\"task_id\": \"abc123\", \"summary\": \"Implemented JWT middleware with token validation and claim extraction\", \"findings\": [\"Existing middleware uses chi router chain\", \"Found hardcoded secret in config - moved to env var\"], \"concerns\": [\"No token refresh mechanism yet\", \"Rate limiting needed on auth endpoints\"], \"feedback\": [\"Consider adding a separate task for rate limiting\"], \"files_changed\": [\"middleware/auth.go\", \"middleware/auth_test.go\", \"config/config.go\"]}"
}
```

---

### `submit_handoff`

Подает Handoff документ для subplanner задачи. Доступен только subplanners.

**Parameters JSON Schema:**
```json
{
  "type": "object",
  "properties": {
    "task_id": {
      "type": "string",
      "description": "ID of the subplan task being completed"
    },
    "summary": {
      "type": "string",
      "description": "Aggregate summary of all sub-tasks"
    },
    "findings": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Aggregated findings from sub-tasks"
    },
    "concerns": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Aggregated concerns from sub-tasks"
    },
    "feedback": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Aggregated feedback from sub-tasks"
    },
    "files_changed": {
      "type": "array",
      "items": {"type": "string"},
      "description": "All files changed across sub-tasks"
    }
  },
  "required": ["task_id", "summary"]
}
```

---

## File Tools (`internal/tool/file.go`)

Все file tools работают относительно `config.Tools.WorkDir`.

### `read_file`

Читает содержимое файла.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Relative path to the file"
    }
  },
  "required": ["path"]
}
```

**Возвращает:** Содержимое файла с пронумерованными строками (формат `   N: line`) или сообщение об ошибке. Нумерация строк используется инструментом `replace_lines` для указания диапазона замены.

**Ограничения:**
- Путь резолвится относительно `work_dir`
- Запрещен выход за пределы `work_dir` (path traversal protection)
- Максимальный размер файла для чтения: 1MB (возвращает ошибку для больших файлов)

---

### `write_file`

Записывает содержимое в файл. Создает директории при необходимости.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Relative path to the file"
    },
    "content": {
      "type": "string",
      "description": "File content to write"
    }
  },
  "required": ["path", "content"]
}
```

**Возвращает:** `"File written: {path} ({bytes} bytes)"`

**Ограничения:**
- Path traversal protection (как read_file)
- Создает промежуточные директории (`os.MkdirAll`)

---

### `list_dir`

Список файлов и директорий.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Relative path to directory. Empty string or '.' for work_dir root."
    }
  },
  "required": ["path"]
}
```

**Возвращает:** Список записей, по одной на строку. Директории помечены суффиксом `/`.

```
cmd/
internal/
go.mod
go.sum
main.go
```

---

### `edit_file`

Редактирует файл, заменяя точный фрагмент текста. Более безопасная альтернатива `write_file` для точечных изменений — не нужно переписывать весь файл.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Relative path to the file"
    },
    "old_text": {
      "type": "string",
      "description": "Exact text to find and replace (must match exactly, including whitespace)"
    },
    "new_text": {
      "type": "string",
      "description": "Text to replace the old text with"
    }
  },
  "required": ["path", "old_text", "new_text"]
}
```

**Возвращает:** `"File edited: {path}"` или сообщение об ошибке если `old_text` не найден.

**Ограничения:**
- `old_text` должен точно совпадать (включая пробелы и переносы строк)
- Path traversal protection (как `read_file`)

---

### `replace_lines`

Заменяет диапазон строк в файле по номерам. Удобен когда нужно заменить блок кода, зная его позицию.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Relative path to the file"
    },
    "start_line": {
      "type": "integer",
      "description": "First line number to replace (1-indexed, inclusive)"
    },
    "end_line": {
      "type": "integer",
      "description": "Last line number to replace (1-indexed, inclusive)"
    },
    "new_content": {
      "type": "string",
      "description": "New content to replace the specified lines"
    }
  },
  "required": ["path", "start_line", "end_line", "new_content"]
}
```

**Возвращает:** `"Lines {start}-{end} replaced in {path}"` или ошибку.

**Ограничения:**
- Path traversal protection (как `read_file`)
- `start_line` <= `end_line`, оба в пределах файла

---

## Shell Tools (`internal/tool/shell.go`)

### `shell_exec`

Выполняет shell-команду через `runner.AgentRunner`.

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "Shell command to execute"
    }
  },
  "required": ["command"]
}
```

**Возвращает:** stdout + stderr output команды. При ненулевом exit code -- `"Exit code: {code}\n{output}"`.

**Ограничения:**
- Если `config.Tools.AllowedShell` не пустой, первое слово команды проверяется по whitelist
- Working directory = `config.Tools.WorkDir`
- Timeout: наследуется от context (глобальный timeout оркестратора)

**Кросс-платформенность:**
- Unix: `sh -c "{command}"` через PTY
- Windows: `cmd /C "{command}"` через pipes

---

## Git Tools (`internal/tool/git.go`)

Доступны только если `config.Tools.GitEnabled = true`. Все команды выполняются в `config.Tools.WorkDir`.

### `git_status`

**Parameters:**
```json
{
  "type": "object",
  "properties": {},
  "required": []
}
```

**Возвращает:** Output `git status --porcelain`.

---

### `git_diff`

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "staged": {
      "type": "boolean",
      "description": "If true, show staged changes (--cached). Default: false"
    },
    "path": {
      "type": "string",
      "description": "Optional path filter"
    }
  },
  "required": []
}
```

**Возвращает:** Output `git diff [--cached] [path]`.

---

### `git_commit`

**Parameters:**
```json
{
  "type": "object",
  "properties": {
    "message": {
      "type": "string",
      "description": "Commit message"
    },
    "files": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Files to stage (git add). Empty = stage all modified."
    }
  },
  "required": ["message"]
}
```

**Выполняет:**
1. `git add {files}` (или `git add -A` если files пустой)
2. `git commit -m "{message}"`

**Возвращает:** Output git commit или сообщение об ошибке.

---

## Безопасность Tools

### Path Traversal Protection

File tools проверяют, что resolved path находится внутри `work_dir`:

```go
func safePath(workDir, relPath string) (string, error) {
    abs := filepath.Join(workDir, filepath.Clean(relPath))
    // Проверяем что abs начинается с workDir
    if !strings.HasPrefix(abs, filepath.Clean(workDir)) {
        return "", fmt.Errorf("path traversal detected: %s", relPath)
    }
    return abs, nil
}
```

### Shell Command Whitelist

Если `allowed_shell` не пустой:

```go
func isAllowed(command string, allowed []string) bool {
    parts := strings.Fields(command)
    if len(parts) == 0 {
        return false
    }
    cmd := filepath.Base(parts[0])
    for _, a := range allowed {
        if cmd == a {
            return true
        }
    }
    return false
}
```

### Tool Error Handling

Tools возвращают ошибки как текстовые строки, не как Go errors. Это позволяет LLM "увидеть" ошибку и отреагировать:

```go
func (t *ReadFileTool) Execute(ctx context.Context, args string) (string, error) {
    // ...
    data, err := os.ReadFile(path)
    if err != nil {
        return fmt.Sprintf("Error reading file: %s", err), nil // не error!
    }
    return string(data), nil
}
```

Go error возвращается только при критических ситуациях (context cancelled, panic). Ожидаемые ошибки (файл не найден, permission denied) возвращаются как строка в первом return value.
