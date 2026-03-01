# Конфигурация Code-Agents

## Формат конфигурации

Конфигурация задается в YAML-файле. По умолчанию ищется `code-agents.yaml` в текущей директории. Путь можно указать через флаг `-c`/`--config`.

## Полная схема

```yaml
# Версия формата конфигурации
version: 1

# OpenAI-compatible LLM провайдер
provider:
  # Base URL эндпоинта (без /chat/completions)
  base_url: "https://your-provider.com/v1"

  # API ключ. Поддерживает env: префикс для чтения из переменной окружения
  api_key: "env:CODEAGENTS_API_KEY"

# Настройки агентов по ролям
agents:
  # Root Planner -- стратегическая декомпозиция
  planner:
    model:
      model: "qwen3-235b-a22b"       # название модели для провайдера
      temperature: 0.3                 # низкая для стабильности планирования
      max_tokens: 4096                 # лимит токенов на ответ
    system_prompt: |
      You are a root planner for a self-driving codebase system.
      ...

  # Subplanner -- тактическая декомпозиция узкого скоупа
  subplanner:
    max_history_messages: 50     # максимум сообщений в истории агента
    model:
      model: "qwen3-235b-a22b"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: |
      You are a subplanner responsible for a narrow slice of work.
      ...

  # Worker -- непосредственное исполнение задач
  worker:
    model:
      model: "qwen3-32b"              # можно использовать более легкую модель
      temperature: 0.2
      max_tokens: 8192                 # больше токенов для генерации кода
    system_prompt: |
      You are a worker agent. You receive a specific task with constraints.
      ...

# Настройки оркестрации
loop:
  max_depth: 3             # максимальная глубина рекурсии subplanners
  max_workers: 4           # количество параллельных worker goroutines
  max_steps: 30            # максимум итераций planner loop
  max_retries: 2           # повторных попыток при ошибке агента
  timeout: "30m"           # глобальный таймаут (Go duration format)
  step_delay: "2s"         # пауза между итерациями planner loop
  retry_delay: "5s"        # задержка перед повторной попыткой
  auto_approve: false      # автоматически принимать все задачи

# Настройки инструментов
tools:
  work_dir: "."            # базовая рабочая директория
  allowed_shell: []        # разрешенные shell-команды (пустой = все)
  git_enabled: true        # включить git tools

# Входные данные
input:
  prompt: "file:./task.md" # задание: строка или file:путь
```

## Секция `provider`

Настройки LLM-провайдера. Используется один HTTP-клиент для всех агентов.

### `base_url`

Base URL OpenAI-compatible API. Клиент отправляет POST на `{base_url}/chat/completions`.

Примеры:
```yaml
# OpenAI
base_url: "https://api.openai.com/v1"

# Свой сервер
base_url: "http://localhost:8080/v1"

# OpenRouter
base_url: "https://openrouter.ai/api/v1"

# Azure OpenAI
base_url: "https://your-resource.openai.azure.com/openai/deployments/your-deployment"
```

### `api_key`

API ключ для авторизации. Передается как `Authorization: Bearer {api_key}`.

Два формата:
```yaml
# Прямое значение (не рекомендуется для production)
api_key: "sk-abc123..."

# Из переменной окружения (рекомендуется)
api_key: "env:CODEAGENTS_API_KEY"
```

При использовании `env:` префикса значение читается из указанной переменной окружения при загрузке конфига. Ошибка, если переменная не задана.

## Секция `agents`

Настройки для каждого типа агента. Каждый тип имеет свою модель и system prompt.

### `model`

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `model` | string | обязательное | Название модели для провайдера |
| `temperature` | float64 | 0.3 | Температура генерации (0.0 - 2.0) |
| `max_tokens` | int | 4096 | Максимум токенов в ответе |

Разные модели для разных ролей -- ключевое преимущество. Planner требует сильной модели для стратегического мышления. Worker может использовать более быструю/дешевую модель для конкретных задач.

### `max_history_messages`

Максимальное количество сообщений в истории диалога агента. По умолчанию `50`. При достижении лимита старые сообщения обрезаются, чтобы контекст не переполнял окно модели.

```yaml
agents:
  planner:
    max_history_messages: 50   # по умолчанию
  worker:
    max_history_messages: 30   # сократить для быстрых worker-задач
```

### `provider` (per-agent override)

Каждый агент может использовать **свой** LLM-провайдер независимо от глобального `provider`. Это позволяет запускать разные модели с разных серверов или провайдеров:

```yaml
provider:
  base_url: "http://localhost:8080/v1"   # глобальный (для planner)
  api_key: "env:LLM_API_KEY"

agents:
  worker:
    provider:                            # переопределение только для worker
      base_url: "http://localhost:8082/v1"
      api_key: "env:WORKER_API_KEY"
    model:
      model: "qwen2.5-coder-14b"
```

Если `agents.*.provider` не задан, используется глобальный `provider`.

### `system_prompt`

System prompt агента. Два формата:

```yaml
# Inline (многострочный YAML)
system_prompt: |
  You are a root planner for a self-driving codebase system.
  You MUST NOT write code directly.
  Break the work into tasks using create_task.

# Из файла
system_prompt: "file:./prompts/planner.md"
```

#### Рекомендуемый system prompt для Planner

```
You are a root planner for a self-driving codebase system.

YOUR RESPONSIBILITIES:
- Own the full scope of the user's request
- Break work into well-scoped tasks via create_task
- Monitor handoffs from completed tasks
- Reassess and create follow-up tasks when needed

CONSTRAINTS:
- You MUST NOT write code directly
- No TODOs, no partial implementations in task descriptions
- Each task must have clear scope and constraints
- Mark tasks as is_subplan=true when scope is too large for one worker
- When all work is validated and complete, output CODEAGENTS_DONE
```

#### Рекомендуемый system prompt для Subplanner

```
You are a subplanner responsible for a narrow slice of work.

YOUR RESPONSIBILITIES:
- Understand your assigned scope completely
- Break your scope into actionable worker tasks
- Monitor handoffs from your workers
- Create follow-up tasks if needed
- Submit aggregate handoff when your scope is complete

CONSTRAINTS:
- You MUST NOT write code directly
- Stay within your assigned scope
- Do not create tasks outside your scope boundary
```

#### Рекомендуемый system prompt для Worker

```
You are a worker agent executing a specific coding task.

YOUR RESPONSIBILITIES:
- Read and understand the task description and constraints
- Implement the required changes using available tools
- Test your changes when possible
- Submit a handoff document on completion

CONSTRAINTS:
- Work only within the scope defined in your task
- Do not modify files outside your scope
- Report concerns and findings honestly
- No partial implementations -- finish what you start
```

## Секция `loop`

Настройки оркестрационного loop (вдохновлены clancy).

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `max_depth` | int | 3 | Максимальная глубина рекурсии subplanners. Ограничивает иерархию: 1 = только planner+workers, 2 = один уровень subplanners, и т.д. |
| `max_workers` | int | 4 | Количество параллельных worker goroutines. Определяет степень параллелизма исполнения. |
| `max_steps` | int | 30 | Максимум итераций root planner loop. Защита от бесконечной работы. |
| `timeout` | string | "30m" | Глобальный таймаут в формате Go duration. По истечении -- cancel всех goroutines. |
| `step_delay` | string | "2s" | Пауза между итерациями planner loop. Дает время workers завершить задачи. |
| `max_retries` | int | 2 | Макс. повторных попыток при ошибке агента (tool call fail, API error). |
| `retry_delay` | string | "5s" | Задержка перед повторной попыткой агента. |
| `auto_approve` | bool | false | Автоматически принимать все задачи без подтверждения пользователя. |

### Duration format

Используется стандартный Go `time.ParseDuration`:
- `"30m"` -- 30 минут
- `"1h30m"` -- полтора часа
- `"5s"` -- 5 секунд
- `"0"` -- без задержки

## Секция `tools`

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `work_dir` | string | "." | Базовая директория для file tools. Все пути резолвятся относительно нее. |
| `allowed_shell` | []string | [] (все) | Whitelist разрешенных shell-команд. Пустой массив = разрешены все. |
| `git_enabled` | bool | true | Включить git tools (git_status, git_diff, git_commit). |

### Ограничение shell-команд

```yaml
# Разрешить только определенные команды
tools:
  allowed_shell:
    - "go"
    - "npm"
    - "git"
    - "make"

# Разрешить все (по умолчанию)
tools:
  allowed_shell: []
```

Если `allowed_shell` не пустой, `shell_exec` проверяет первое слово команды и отклоняет неразрешенные.

## Секция `input`

| Поле | Тип | По умолчанию | Описание |
|------|-----|-------------|----------|
| `prompt` | string | обязательное | Задание для root planner. Строка или `file:путь`. |

```yaml
# Inline задание
input:
  prompt: "Refactor the authentication module to use JWT tokens"

# Из файла
input:
  prompt: "file:./prompts/refactor-auth.md"
```

## Полный пример конфигурации

```yaml
version: 1

provider:
  base_url: "http://localhost:11434/v1"
  api_key: "env:LLM_API_KEY"

agents:
  planner:
    model:
      model: "qwen3-235b-a22b"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "file:./prompts/planner.md"

  subplanner:
    model:
      model: "qwen3-235b-a22b"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "file:./prompts/subplanner.md"

  worker:
    model:
      model: "qwen3-32b"
      temperature: 0.2
      max_tokens: 8192
    system_prompt: "file:./prompts/worker.md"

loop:
  max_depth: 2
  max_workers: 3
  max_steps: 20
  timeout: "15m"
  step_delay: "3s"

tools:
  work_dir: "/home/user/project"
  allowed_shell: ["go", "git", "make"]
  git_enabled: true

input:
  prompt: "file:./task.md"
```

## Валидация

При загрузке конфигурации проверяется:

1. `version` == 1
2. `provider.base_url` -- не пустой, валидный URL
3. `provider.api_key` -- не пустой; если `env:`, переменная существует
4. `agents.*.model.model` -- не пустой для каждой роли
5. `agents.*.system_prompt` -- не пустой; если `file:`, файл существует
6. `loop.timeout` и `loop.step_delay` -- парсятся как `time.Duration`
7. `loop.max_workers` >= 1
8. `loop.max_steps` >= 1
9. `input.prompt` -- не пустой; если `file:`, файл существует

При ошибке валидации -- немедленный выход с понятным сообщением об ошибке.
