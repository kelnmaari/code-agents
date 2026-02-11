# Подбор GGUF моделей для 2x RTX 4090

## Железо

- 2x NVIDIA RTX 4090 (24 GB VRAM каждая, 48 GB суммарно)
- Инференс через [llama.cpp](https://github.com/ggml-org/llama.cpp) (`llama-server`)
- 3 инстанса `llama-server` на разных портах (по одному на роль агента)
- OpenAI-compatible API на каждом порту

## Общие требования к моделям

- **GGUF** формат
- Поддержка **tool/function calling** через `llama-server --jinja`
- Квантизация **Q4_K_M** или **Q5_K_M** (баланс качества и размера)
- VRAM = размер файла + KV cache (~2-4 GB при ctx_size 8192)

---

## Вариант A: Qwen3 экосистема (сбалансированный)

| Роль | Модель | Параметры | Квант | Файл | VRAM* | GPU |
|------|--------|-----------|-------|------|-------|-----|
| **Planner** | Qwen3-14B | 14B dense | Q4_K_M | ~9 GB | ~12 GB | GPU 0 |
| **Subplanner** | Qwen3-8B | 8B dense | Q4_K_M | ~5 GB | ~7 GB | GPU 1 |
| **Worker** | Qwen2.5-Coder-14B-Instruct | 14B dense | Q4_K_M | ~8.9 GB | ~11 GB | GPU 1 |
| | | | **Итого файлы:** | **~23 GB** | **~30 GB** | |

**Характеристика:**
- Planner (Qwen3-14B) -- thinking mode, сильное reasoning, native tool calling
- Subplanner (Qwen3-8B) -- та же архитектура Qwen3, достаточно для декомпозиции задач
- Worker (Qwen2.5-Coder-14B) -- специализированная coding модель, SOTA среди open-source coders этого размера
- Запас ~18 GB VRAM для KV cache и пиковых нагрузок

**HuggingFace:**
- [unsloth/Qwen3-14B-GGUF](https://huggingface.co/unsloth/Qwen3-14B-GGUF)
- [Qwen/Qwen3-8B-GGUF](https://huggingface.co/Qwen/Qwen3-8B-GGUF)
- [Qwen/Qwen2.5-Coder-14B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-14B-Instruct-GGUF)

---

## Вариант B: Максимум Worker (Devstral)

| Роль | Модель | Параметры | Квант | Файл | VRAM* | GPU |
|------|--------|-----------|-------|------|-------|-----|
| **Planner** | Qwen3-14B | 14B dense | Q4_K_M | ~9 GB | ~12 GB | GPU 0 |
| **Subplanner** | Qwen3-4B | 4B dense | Q5_K_M | ~2.9 GB | ~4 GB | GPU 0 |
| **Worker** | Devstral-Small-2507 | 24B dense | Q4_K_M | ~14.3 GB | ~18 GB | GPU 1 |
| | | | **Итого файлы:** | **~26 GB** | **~34 GB** | |

**Характеристика:**
- Worker (Devstral-24B) -- модель от Mistral, специально заточенная под agentic coding, 128k контекст
- Subplanner ужат до 4B, чтобы освободить VRAM для мощного Worker
- Planner + Subplanner на GPU 0 (~16 GB), Worker целиком на GPU 1 (~18 GB)

**HuggingFace:**
- [unsloth/Qwen3-14B-GGUF](https://huggingface.co/unsloth/Qwen3-14B-GGUF)
- [Qwen/Qwen3-4B-GGUF](https://huggingface.co/Qwen/Qwen3-4B-GGUF)
- [unsloth/Devstral-Small-2507-GGUF](https://huggingface.co/unsloth/Devstral-Small-2507-GGUF)

---

## Вариант C: MoE Planner (Qwen3-Coder-30B-A3B)

| Роль | Модель | Параметры | Квант | Файл | VRAM* | GPU |
|------|--------|-----------|-------|------|-------|-----|
| **Planner** | Qwen3-Coder-30B-A3B-Instruct | 30B total / 3B active (MoE) | Q4_K_M | ~18.6 GB | ~20 GB | GPU 0 |
| **Subplanner** | Qwen3-4B | 4B dense | Q4_K_M | ~2.5 GB | ~4 GB | GPU 1 |
| **Worker** | Qwen2.5-Coder-14B-Instruct | 14B dense | Q4_K_M | ~8.9 GB | ~11 GB | GPU 1 |
| | | | **Итого файлы:** | **~30 GB** | **~35 GB** | |

**Характеристика:**
- Planner (Qwen3-Coder-30B-A3B) -- MoE: 128 экспертов, 3B активных на токен. Быстрый как 3B, умный как гораздо крупнее
- Специально обучен для agentic coding и tool calling
- Файл 18.6 GB -- все 30B весов должны быть в памяти, хотя активируется только 3B
- Subplanner + Worker на GPU 1 (~15 GB), Planner целиком на GPU 0 (~20 GB)

**HuggingFace:**
- [unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF](https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF)
- [Qwen/Qwen3-4B-GGUF](https://huggingface.co/Qwen/Qwen3-4B-GGUF)
- [Qwen/Qwen2.5-Coder-14B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-14B-Instruct-GGUF)

---

## Вариант D: Бюджетный (одна 4090)

| Роль | Модель | Параметры | Квант | Файл | VRAM* | GPU |
|------|--------|-----------|-------|------|-------|-----|
| **Planner** | Qwen3-8B | 8B dense | Q4_K_M | ~5 GB | ~7 GB | GPU 0 |
| **Subplanner** | Qwen3-4B | 4B dense | Q4_K_M | ~2.5 GB | ~4 GB | GPU 0 |
| **Worker** | Qwen2.5-Coder-7B-Instruct | 7B dense | Q5_K_M | ~5.4 GB | ~7 GB | GPU 0 |
| | | | **Итого файлы:** | **~13 GB** | **~18 GB** | |

**Характеристика:**
- Всё помещается на одну RTX 4090 (24 GB), вторая свободна для экспериментов
- Минимальный порог входа для тестирования архитектуры
- Qwen2.5-Coder-7B -- уступает 14B, но всё ещё сильная coding модель
- Можно повысить кванты до Q6_K/Q8_0 при таком запасе VRAM

**HuggingFace:**
- [Qwen/Qwen3-8B-GGUF](https://huggingface.co/Qwen/Qwen3-8B-GGUF)
- [Qwen/Qwen3-4B-GGUF](https://huggingface.co/Qwen/Qwen3-4B-GGUF)
- [Qwen/Qwen2.5-Coder-7B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF)

---

## Вариант E: Протестированный (qwen3-8b + SWE-Dev-32B)

| Роль | Модель | Параметры | Квант | GPU |
|------|--------|-----------|-------|-----|
| **Planner** | qwen3-8b-gguf | 8B dense | GGUF | GPU 0 |
| **Subplanner** | qwen3-8b-gguf | 8B dense | GGUF | GPU 0 |
| **Worker** | swe-dev-32b-i1-gguf | 32B dense | IQ1 | GPU 1 |

**Характеристика:**
- Протестированная рабочая конфигурация
- Planner и Subplanner используют одну модель (Qwen3-8B) — достаточно для декомпозиции задач и координации
- Worker использует SWE-Dev-32B (importance-1 quant) — специализированная модель для SWE задач, сильное code generation
- Все три роли работают через один `llama-server` endpoint

> **Примечание:** Это конфигурация, на которой система была реально протестирована и отлажена.

---

## Сводная таблица

| Вариант | Planner | Subplanner | Worker | Файлы | VRAM | GPU layout |
|---------|---------|------------|--------|-------|------|------------|
| **A** | Qwen3-14B | Qwen3-8B | Qwen2.5-Coder-14B | 23 GB | 30 GB | 0: planner / 1: sub+worker |
| **B** | Qwen3-14B | Qwen3-4B | Devstral-24B | 26 GB | 34 GB | 0: planner+sub / 1: worker |
| **C** | Qwen3-Coder-30B-A3B | Qwen3-4B | Qwen2.5-Coder-14B | 30 GB | 35 GB | 0: planner / 1: sub+worker |
| **D** | Qwen3-8B | Qwen3-4B | Qwen2.5-Coder-7B | 13 GB | 18 GB | 0: все три |
| **E** ⭐ | qwen3-8b-gguf | qwen3-8b-gguf | swe-dev-32b-i1-gguf | — | — | протестировано |

---

## Запуск llama-server

### Общие флаги

```bash
# Обязательные для tool calling:
--jinja                    # поддержка Jinja2 шаблонов (нужно для tool calling)
--flash-attn               # Flash Attention (ускорение + экономия VRAM)
-ngl 99                    # все слои на GPU

# Рекомендуемые:
--ctx-size 8192            # контекст (увеличить при необходимости)
--parallel 2               # параллельные запросы (если VRAM позволяет)
--split-mode none          # отключить split между GPU
--main-gpu N               # привязка к конкретному GPU
```

### Вариант A: пример запуска

```bash
# GPU 0: Planner — Qwen3-14B (порт 8080)
llama-server \
  --model Qwen3-14B-Q4_K_M.gguf \
  --port 8080 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0 \
  --temp 0.3 --top-p 0.8 --top-k 20 --repeat-penalty 1.05

# GPU 1: Subplanner — Qwen3-8B (порт 8081)
llama-server \
  --model Qwen3-8B-Q4_K_M.gguf \
  --port 8081 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1 \
  --temp 0.3 --top-p 0.8 --top-k 20 --repeat-penalty 1.05

# GPU 1: Worker — Qwen2.5-Coder-14B (порт 8082)
llama-server \
  --model qwen2.5-coder-14b-instruct-q4_k_m.gguf \
  --port 8082 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1 \
  --temp 0.2 --top-p 0.8 --top-k 20
```

### Вариант B: пример запуска

```bash
# GPU 0: Planner — Qwen3-14B (порт 8080)
llama-server \
  --model Qwen3-14B-Q4_K_M.gguf \
  --port 8080 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

# GPU 0: Subplanner — Qwen3-4B (порт 8081)
llama-server \
  --model Qwen3-4B-Q5_K_M.gguf \
  --port 8081 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

# GPU 1: Worker — Devstral-Small-2507 (порт 8082)
llama-server \
  --model Devstral-Small-2507-Q4_K_M.gguf \
  --port 8082 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1
```

### Вариант C: пример запуска

```bash
# GPU 0: Planner — Qwen3-Coder-30B-A3B (порт 8080)
llama-server \
  --model Qwen3-Coder-30B-A3B-Instruct-Q4_K_M.gguf \
  --port 8080 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

# GPU 1: Subplanner — Qwen3-4B (порт 8081)
llama-server \
  --model Qwen3-4B-Q4_K_M.gguf \
  --port 8081 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1

# GPU 1: Worker — Qwen2.5-Coder-14B (порт 8082)
llama-server \
  --model qwen2.5-coder-14b-instruct-q4_k_m.gguf \
  --port 8082 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 1
```

### Вариант D: пример запуска

```bash
# GPU 0: все три модели
llama-server \
  --model Qwen3-8B-Q4_K_M.gguf \
  --port 8080 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

llama-server \
  --model Qwen3-4B-Q4_K_M.gguf \
  --port 8081 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0

llama-server \
  --model qwen2.5-coder-7b-instruct-q5_k_m.gguf \
  --port 8082 --host 0.0.0.0 \
  --jinja --flash-attn -ngl 99 \
  --ctx-size 8192 --split-mode none --main-gpu 0
```

---

## Конфигурация code-agents.yaml

Endpoint один, модели различаются по имени в поле `model`. Каждый llama-server слушает свой порт, но code-agents может использовать proxy (nginx/llama-swap) перед ними, или конфиг поддерживает per-agent `base_url`:

```yaml
provider:
  base_url: "http://localhost:8080/v1"  # default endpoint
  api_key: "none"                        # llama-server не требует ключ

agents:
  planner:
    model:
      model: "Qwen3-14B"
      temperature: 0.3
      max_tokens: 4096

  subplanner:
    model:
      model: "Qwen3-8B"
      temperature: 0.3
      max_tokens: 4096

  worker:
    model:
      model: "Qwen2.5-Coder-14B-Instruct"
      temperature: 0.2
      max_tokens: 8192
```

---

## Скачивание моделей

```bash
# Установить huggingface-cli если нет
pip install huggingface-hub

# Вариант A
huggingface-cli download unsloth/Qwen3-14B-GGUF Qwen3-14B-Q4_K_M.gguf --local-dir ./models
huggingface-cli download Qwen/Qwen3-8B-GGUF Qwen3-8B-Q4_K_M.gguf --local-dir ./models
huggingface-cli download Qwen/Qwen2.5-Coder-14B-Instruct-GGUF qwen2.5-coder-14b-instruct-q4_k_m.gguf --local-dir ./models

# Вариант B (дополнительно)
huggingface-cli download Qwen/Qwen3-4B-GGUF Qwen3-4B-Q5_K_M.gguf --local-dir ./models
huggingface-cli download unsloth/Devstral-Small-2507-GGUF Devstral-Small-2507-Q4_K_M.gguf --local-dir ./models

# Вариант C (дополнительно)
huggingface-cli download unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF \
  --include "Qwen3-Coder-30B-A3B-Instruct-Q4_K_M*.gguf" --local-dir ./models

# Вариант D (дополнительно)
huggingface-cli download Qwen/Qwen2.5-Coder-7B-Instruct-GGUF qwen2.5-coder-7b-instruct-q5_k_m.gguf --local-dir ./models
```

---

## Полный каталог моделей на HuggingFace

### Qwen3 (reasoning + thinking + tool calling)

| Модель | Параметры | Q4_K_M | Q5_K_M | Q8_0 | Ссылка |
|--------|-----------|--------|--------|------|--------|
| Qwen3-4B | 4B | 2.5 GB | 2.9 GB | 4.3 GB | [Qwen/Qwen3-4B-GGUF](https://huggingface.co/Qwen/Qwen3-4B-GGUF) |
| Qwen3-8B | 8B | 5.0 GB | 5.9 GB | 8.7 GB | [Qwen/Qwen3-8B-GGUF](https://huggingface.co/Qwen/Qwen3-8B-GGUF) |
| Qwen3-14B | 14B | 9.0 GB | — | — | [unsloth/Qwen3-14B-GGUF](https://huggingface.co/unsloth/Qwen3-14B-GGUF) |

### Qwen2.5-Coder (специализированные coding модели)

| Модель | Параметры | Q4_K_M | Q5_K_M | Q8_0 | Ссылка |
|--------|-----------|--------|--------|------|--------|
| Qwen2.5-Coder-7B-Instruct | 7B | ~4.7 GB | ~5.4 GB | ~7.7 GB | [Qwen/Qwen2.5-Coder-7B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-7B-Instruct-GGUF) |
| Qwen2.5-Coder-14B-Instruct | 14B | ~8.9 GB | — | — | [Qwen/Qwen2.5-Coder-14B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-14B-Instruct-GGUF) |
| Qwen2.5-Coder-32B-Instruct | 32B | ~19.9 GB | — | — | [Qwen/Qwen2.5-Coder-32B-Instruct-GGUF](https://huggingface.co/Qwen/Qwen2.5-Coder-32B-Instruct-GGUF) |

### Qwen3-Coder (MoE, agentic coding)

| Модель | Total/Active | Q4_K_M | Ссылка |
|--------|-------------|--------|--------|
| Qwen3-Coder-30B-A3B-Instruct | 30B / 3B | ~18.6 GB | [unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF](https://huggingface.co/unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF) |
| Qwen3-Coder-Next | 80B / 3B | ~40-45 GB | [unsloth/Qwen3-Coder-Next-GGUF](https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF) |

### Devstral (agentic coding от Mistral)

| Модель | Параметры | Q4_K_M | Q5_K_M | Q8_0 | Ссылка |
|--------|-----------|--------|--------|------|--------|
| Devstral-Small-2507 | 24B | ~14.3 GB | ~16.8 GB | ~25.1 GB | [unsloth/Devstral-Small-2507-GGUF](https://huggingface.co/unsloth/Devstral-Small-2507-GGUF) |

---

## Рекомендации

1. **Начните с Варианта D** -- быстро проверить что архитектура работает, минимум VRAM
2. **Перейдите на Вариант A** -- оптимальный баланс для production-like тестирования
3. **Попробуйте Вариант C** -- если MoE даст лучшее качество планирования
4. **Вариант B** -- если Worker является bottleneck и нужна максимальная coding мощь

**Оптимизация VRAM:**
- KV cache quantization: `--cache-type-k q8_0 --cache-type-v q4_0` (экономит ~50% KV cache)
- Уменьшить `--ctx-size` если задачи не требуют длинного контекста
- Flash Attention (`--flash-attn`) обязателен при одновременном запуске 3 моделей
