# Code-Agents

**Многоагентная иерархическая система для автономной работы над кодовыми базами.**

Архитектура вдохновлена исследованием Cursor «Self-Driving Codebases» — три уровня агентов (Planner → Subplanner → Worker) координируются через общую очередь задач. В качестве LLM-бэкенда используется любой OpenAI-compatible API (llama.cpp, Ollama, OpenRouter, и т.д.).

Паттерн loop-оркестратора адаптирован из проекта [clancy](https://github.com/eduardolat/clancy).

---

## Как это работает

```
                    ┌──────────────────┐
                    │   Root Planner    │
                    │ (стратегический)  │
                    └────────┬─────────┘
                             │ создает задачи
                    ┌────────▼─────────┐
              ┌─────┤   Task Queue     ├─────┐
              │     └──────────────────┘     │
              │                              │
     ┌────────▼─────────┐          ┌────────▼─────────┐
     │   Subplanner      │          │     Worker        │
     │ (тактический)     │          │ (исполнитель)     │
     └────────┬─────────┘          └────────┬─────────┘
              │ подзадачи                    │ handoff
              ▼                              ▼
         Task Queue                    Queue.Complete()
```

1. **Root Planner** получает ваш промпт и декомпозирует работу на задачи
2. **Workers** (пул горутин) берут задачи из очереди, пишут код, запускают тесты
3. **Subplanners** обрабатывают слишком крупные задачи, разбивая их на подзадачи
4. Информация течёт **снизу вверх** через Handoff-документы
5. Planner переоценивает ситуацию, создаёт follow-up задачи, и завершает работу сигналом `CODEAGENTS_DONE`

---

## Быстрый старт

### Установка

Скачайте бинарник со [страницы релизов](../../releases) или соберите из исходников:

```bash
go build -o code-agents ./cmd/code-agents
```

### Генерация конфигурации

```bash
code-agents --init
```

Создаст файл `code-agents.yaml` с шаблоном конфигурации.

### Запуск

```bash
code-agents code-agents.yaml
```

---

## Docker

### Сборка образа

```bash
docker build -f Dockerfile.run -t code-agents .
```

### Запуск

```bash
docker run --rm -it \
  -v /path/to/your/project:/workspace \
  -v /path/to/code-agents.yaml:/config/code-agents.yaml:ro \
  -e CODEAGENTS_API_KEY=your-api-key \
  --network host \
  code-agents
```

### Параметры запуска

| Параметр | Описание |
|----------|----------|
| `-v /path/to/project:/workspace` | Рабочая директория проекта (сюда агенты пишут код) |
| `-v /path/to/config.yaml:/config/code-agents.yaml:ro` | Файл конфигурации (read-only) |
| `-e CODEAGENTS_API_KEY=...` | API ключ для LLM провайдера |
| `--network host` | Доступ к локальному LLM серверу (llama.cpp и т.д.) |

### Примеры

```bash
# Запуск с промптом из конфига
docker run --rm -it \
  -v $(pwd):/workspace \
  -v $(pwd)/code-agents.yaml:/config/code-agents.yaml:ro \
  -e CODEAGENTS_API_KEY=sk-xxx \
  --network host \
  code-agents

# Запуск с промптом из CLI
docker run --rm -it \
  -v $(pwd):/workspace \
  -v $(pwd)/code-agents.yaml:/config/code-agents.yaml:ro \
  --network host \
  code-agents -c /config/code-agents.yaml "Create a REST API server"

# Генерация шаблона конфигурации
docker run --rm -v $(pwd):/workspace code-agents --init
```

> **Важно:** В конфигурации `tools.work_dir` должен быть установлен в `.` (по умолчанию), т.к. рабочая директория контейнера — `/workspace`.

## Конфигурация

Конфигурация задаётся в YAML-файле. Путь указывается как позиционный аргумент (по умолчанию — `code-agents.yaml`).

### Минимальный пример

```yaml
version: 1

provider:
  base_url: "http://localhost:8080/v1"
  api_key: "env:LLM_API_KEY"

agents:
  planner:
    model:
      model: "qwen3-14b"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "file:./prompts/planner.md"

  subplanner:
    model:
      model: "qwen3-8b"
      temperature: 0.3
      max_tokens: 4096
    system_prompt: "file:./prompts/subplanner.md"

  worker:
    model:
      model: "qwen2.5-coder-14b"
      temperature: 0.2
      max_tokens: 8192
    system_prompt: "file:./prompts/worker.md"

loop:
  max_depth: 3
  max_workers: 4
  max_steps: 30
  timeout: "30m"
  step_delay: "2s"

tools:
  work_dir: "."
  allowed_shell: []
  git_enabled: true

input:
  prompt: "file:./task.md"
```

### Основные параметры

| Параметр | Описание | По умолчанию |
|----------|----------|-------------|
| `provider.base_url` | URL OpenAI-compatible API | обязательный |
| `provider.api_key` | API ключ (поддерживает `env:VAR_NAME`) | обязательный |
| `loop.max_workers` | Параллельных workers | 4 |
| `loop.max_steps` | Макс. итераций planner loop | 30 |
| `loop.max_retries` | Макс. повторных попыток при ошибках агента | 2 |
| `loop.timeout` | Глобальный таймаут (Go duration) | 30m |
| `loop.step_delay` | Задержка между итерациями planner | 2s |
| `loop.retry_delay` | Задержка перед повторной попыткой | 5s |
| `loop.max_depth` | Глубина рекурсии subplanners | 3 |
| `loop.auto_approve` | Автоматически принимать все задачи | `false` |
| `agents.*.max_history_messages` | Макс. сообщений в истории агента | 50 |
| `agents.*.provider` | Переопределить LLM-провайдер для конкретной роли | — |
| `tools.work_dir` | Рабочая директория | `.` |
| `tools.allowed_shell` | Whitelist shell-команд (пустой = все) | `[]` |
| `tools.git_enabled` | Включить git tools | `true` |

> Подробнее: [docs/configuration.md](docs/configuration.md)

---

## Инструменты агентов

Каждый тип агента имеет свой набор инструментов:

| Инструмент | Planner | Subplanner | Worker |
|------------|:-------:|:----------:|:------:|
| `create_task` | ✓ | ✓ | — |
| `submit_handoff` | — | ✓ | — |
| `complete_task` | — | — | ✓ |
| `read_file` | — | — | ✓ |
| `write_file` | — | — | ✓ |
| `list_dir` | — | — | ✓ |
| `shell_exec` | — | — | ✓ |
| `git_status` | — | — | ✓ |
| `git_diff` | — | — | ✓ |
| `git_commit` | — | — | ✓ |

Planner намеренно лишён доступа к файлам и shell — он только координирует.

> Подробнее: [docs/tools.md](docs/tools.md)

---

## Подбор моделей

Code-Agents рассчитан на локальный инференс через [llama.cpp](https://github.com/ggml-org/llama.cpp). Для каждой роли агента запускается отдельный `llama-server` на своём порту.

### Рекомендуемый вариант (2× RTX 4090)

| Роль | Модель | VRAM | GPU |
|------|--------|------|-----|
| Planner | [Qwen3-14B](https://huggingface.co/unsloth/Qwen3-14B-GGUF) (Q4_K_M) | ~12 GB | GPU 0 |
| Subplanner | [Qwen3-8B](https://huggingface.co/Qwen/Qwen3-8B-GGUF) (Q4_K_M) | ~7 GB | GPU 1 |
| Worker | [Qwen2.5-Coder-14B](https://huggingface.co/Qwen/Qwen2.5-Coder-14B-Instruct-GGUF) (Q4_K_M) | ~11 GB | GPU 1 |

### Пример запуска llama-server

```bash
# Planner (порт 8080, GPU 0)
llama-server \
  --model Qwen3-14B-Q4_K_M.gguf \
  --port 8080 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

# Subplanner (порт 8081, GPU 1)
llama-server \
  --model Qwen3-8B-Q4_K_M.gguf \
  --port 8081 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1

# Worker (порт 8082, GPU 1)
llama-server \
  --model qwen2.5-coder-14b-instruct-q4_k_m.gguf \
  --port 8082 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1
```

> Подробнее о вариантах моделей и конфигурациях: [docs/models.md](docs/models.md)

---

## CLI

```
code-agents [OPTIONS] [PROMPT...]

Positional arguments:
  PROMPT                 prompt text (overrides config prompt)

Options:
  -c, --config PATH      path to configuration file (default: code-agents.yaml)
  --init                 generate a new configuration file
  --profile NAME         named prompt profile to use (loads profiles/<name>.yaml)
  --runs-dir DIR         directory to save run logs (default: runs)
  --version              show version info
  --help, -h             show help

Subcommands:
  compare RUN1 RUN2      compare two run log JSON files (from runs/ directory)
```

### Примеры использования

```bash
# Запуск с конфигом по умолчанию (code-agents.yaml), промпт из конфига
code-agents

# Передать промпт напрямую через CLI (переопределяет config.input.prompt)
code-agents "Add unit tests for the auth module"

# Явно указать путь к конфигу
code-agents -c /path/to/my-config.yaml "Refactor the database layer"

# Использовать именованный профиль (profiles/backend.yaml)
code-agents --profile backend "Fix the API rate limiting bug"

# Сгенерировать шаблон конфигурации
code-agents --init

# Сравнить два прогона
code-agents compare runs/run-001.json runs/run-002.json
```

---

## Принципы проектирования

- **Anti-fragility** — падение одного агента не роняет систему; planner создаёт замену
- **Throughput > Correctness** — стабильный поток задач важнее идеальных коммитов
- **Constraints over Instructions** — system prompts определяют *границы*, а не пошаговые инструкции
- **Information Flows Upward** — никакого shared state, только Handoff-документы вверх по иерархии
- **No Static Plans** — planner непрерывно переоценивает ситуацию на основе результатов workers

> Подробнее: [docs/architecture.md](docs/architecture.md)

---

## Структура проекта

```
code-agents/
├── cmd/code-agents/        # CLI — точка входа
├── internal/
│   ├── config/             # загрузка YAML, валидация
│   ├── llm/                # OpenAI-compatible HTTP клиент
│   ├── agent/              # абстракция агента с tool-use loop
│   ├── tool/               # реализации инструментов (file, shell, git, task)
│   ├── task/               # Task, Handoff, thread-safe Queue
│   ├── orchestrator/       # координация planner + worker pool
│   ├── runner/             # кросс-платформенный shell (PTY/pipes)
│   ├── runlog/             # метрики и логи запусков (JSON, сравнение прогонов)
│   ├── logging/            # инициализация логгеров (file + console)
│   └── version/            # build-time версия
├── prompts/                # примеры промптов для задач
├── docs/                   # документация
└── code-agents.yaml        # пример конфигурации
```

> Подробнее: [docs/project-structure.md](docs/project-structure.md)

---

## Документация

| Документ | Описание |
|----------|----------|
| [architecture.md](docs/architecture.md) | Архитектура, иерархия агентов, поток данных |
| [configuration.md](docs/configuration.md) | Полная схема конфигурации с примерами |
| [tools.md](docs/tools.md) | Описание всех инструментов агентов |
| [models.md](docs/models.md) | Подбор GGUF моделей для локального инференса |
| [models-vllm.md](docs/models-vllm.md) | Запуск моделей через vLLM |
| [interfaces.md](docs/interfaces.md) | Ключевые интерфейсы и типы Go |
| [orchestration-loop.md](docs/orchestration-loop.md) | Пошаговое описание оркестрации |
| [project-structure.md](docs/project-structure.md) | Структура проекта и назначение пакетов |
| [prompt-measurement.md](docs/prompt-measurement.md) | Методология измерения качества промптов |

---

## Разработка и тесты

```bash
# Запуск тестов
go test -race -timeout 120s ./...

# Проверка кода
go vet ./...
```

---

## Сборка из исходников

```bash
# Обычная сборка
go build -o code-agents ./cmd/code-agents

# С версией
go build -ldflags "-s -w \
  -X gitlab.alexue4.dev/kelnmaari/code-agent/internal/version.Version=1.0.0 \
  -X gitlab.alexue4.dev/kelnmaari/code-agent/internal/version.Commit=$(git rev-parse HEAD)" \
  -o code-agents ./cmd/code-agents

# Кросс-компиляция
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o code-agents-linux-amd64 ./cmd/code-agents
```

---

## Зависимости

| Пакет | Назначение |
|-------|------------|
| [go-arg](https://github.com/alexflint/go-arg) | Парсинг CLI аргументов |
| [yaml.v3](https://gopkg.in/yaml.v3) | Парсинг YAML конфигурации |
| [go-nanoid](https://github.com/matoous/go-nanoid) | Генерация ID для агентов и задач |
| [pty](https://github.com/creack/pty) | PTY для Unix shell execution |
| [testify](https://github.com/stretchr/testify) | Assertions в тестах |

---

## Лицензия

See [LICENSE](LICENSE) for details.
