# Модели для vLLM

Конфигурация моделей для запуска через [vLLM](https://docs.vllm.ai/) с поддержкой **function calling / tool use**.

> **Формат:** vLLM работает с нативным HuggingFace форматом (`hf`), GGUF не требуется.

---

## Железо

- 2× NVIDIA RTX 4090 (24 GB VRAM каждая, 48 GB суммарно)
- Инференс через vLLM (OpenAI-compatible API)

---

## Рекомендуемая конфигурация (протестировано)

| Роль | HF Repo | Параметры | Tool Calling Parser | GPU |
|------|---------|-----------|---------------------|-----|
| **Planner** | `Qwen/Qwen3-8B` | 8B dense | `hermes` | GPU 0 |
| **Subplanner** | `Qwen/Qwen3-8B` | 8B dense | `hermes` | GPU 0 |
| **Worker** | `zai-org/SWE-Dev-32B` | 32B dense | `hermes` | GPU 1 |

**Характеристика:**
- Planner/Subplanner используют одну модель — Qwen3-8B с native tool calling
- Worker — SWE-Dev-32B, специализированная модель для SWE задач (на базе Qwen2.5-Coder-32B-Instruct)
- Qwen3-8B (~16 GB с KV cache) на GPU 0, SWE-Dev-32B (~22 GB) на GPU 1

---

## Альтернативные Worker-модели

Все модели ниже основаны на `Qwen2.5-Coder-32B-Instruct` и подходят для роли Worker:

| Модель | HF Repo | Описание | VRAM (fp16) |
|--------|---------|----------|-------------|
| **SWE-Dev-32B** | [`zai-org/SWE-Dev-32B`](https://huggingface.co/zai-org/SWE-Dev-32B) | SWE-задачи, reasoning + tool use | ~64 GB fp16 / ~22 GB AWQ |
| **OpenHands-LM-32B** | [`all-hands/openhands-lm-32b-v0.1`](https://huggingface.co/all-hands/openhands-lm-32b-v0.1) | Agentic coding, SWE-Bench Verified | ~64 GB fp16 |
| **Skywork-SWE-32B** | [`Skywork/Skywork-SWE-32B`](https://huggingface.co/Skywork/Skywork-SWE-32B) | SWE-agent, код-редактирование | ~64 GB fp16 |
| **SWE-Swiss-32B** | [`SWE-Swiss/SWE-Swiss-32B`](https://huggingface.co/SWE-Swiss/SWE-Swiss-32B) | Исправление issues, SWE-бенчмарки | ~64 GB fp16 |
| **Devstral-Small-2-24B** | [`mistralai/Devstral-Small-2-24B-Instruct-2512`](https://huggingface.co/mistralai/Devstral-Small-2-24B-Instruct-2512) | Agentic coding от Mistral, 256k контекст | ~48 GB fp16 |

> **Важно:** 32B модели в fp16 (~64 GB) не помещаются на одну RTX 4090 (24 GB).  
> Используйте **квантизацию AWQ/GPTQ** (4-bit → ~20 GB) или **Tensor Parallel** на 2 GPU.

### Квантизованные версии (помещаются на одну 4090)

| Модель | HF Repo | Квант | Размер |
|--------|---------|-------|--------|
| SWE-Dev-32B AWQ | [`casperhansen/SWE-Dev-32B-AWQ`](https://huggingface.co/casperhansen/SWE-Dev-32B-AWQ) | AWQ 4-bit | ~18 GB |
| OpenHands-LM-32B AWQ | [`all-hands/openhands-lm-32b-v0.1-AWQ`](https://huggingface.co/all-hands/openhands-lm-32b-v0.1-AWQ) | AWQ 4-bit | ~18 GB |
| Qwen2.5-Coder-32B GPTQ | [`Qwen/Qwen2.5-Coder-32B-Instruct-GPTQ-Int4`](https://huggingface.co/Qwen/Qwen2.5-Coder-32B-Instruct-GPTQ-Int4) | GPTQ 4-bit | ~18 GB |

---

## Альтернативные Planner-модели

| Модель | HF Repo | Описание | VRAM (fp16) |
|--------|---------|----------|-------------|
| **Qwen3-8B** ⭐ | [`Qwen/Qwen3-8B`](https://huggingface.co/Qwen/Qwen3-8B) | Thinking mode, native tool calling | ~16 GB |
| **Qwen3-14B** | [`Qwen/Qwen3-14B`](https://huggingface.co/Qwen/Qwen3-14B) | Сильнее в reasoning, 14B dense | ~28 GB fp16 |
| **Qwen3-4B** | [`Qwen/Qwen3-4B`](https://huggingface.co/Qwen/Qwen3-4B) | Компактная, хватит для subplanner | ~8 GB |

---

## Параметры запуска vLLM

### Обязательные для tool calling

Для работы function calling/tool use в vLLM нужны следующие параметры:

| Параметр | Значение | Описание |
|----------|----------|----------|
| `--enable-auto-tool-choice` | — | Включает автоматический выбор tool_choice |
| `--tool-call-parser` | `hermes` | Парсер для Qwen3 / Qwen2.5 моделей |
| `--reasoning-parser` | `deepseek_r1` | Парсер для thinking mode (Qwen3) |

### Рекомендуемые

| Параметр | Значение | Описание |
|----------|----------|----------|
| `--max-model-len` | `8192` | Максимальная длина контекста |
| `--gpu-memory-utilization` | `0.9` | Утилизация GPU (можно поднять с 0.8 до 0.9) |
| `--tensor-parallel-size` | `2` | Для 32B fp16: разделить на 2 GPU |

### Настройка в вашем лаунчере

На основе скриншота лаунчера, вот что нужно настроить:

| Поле лаунчера | Значение для Planner | Значение для Worker |
|---------------|---------------------|---------------------|
| **Alias** | `qwen3-8b` | `swe-dev-32b` |
| **Provider** | `vllm` | `vllm` |
| **Format** | `hf` | `hf` |
| **HF Repo** | `Qwen/Qwen3-8B` | `zai-org/SWE-Dev-32B` |
| **GPU Device** | GPU 0 | GPU 1 |
| **Capabilities** | `chat`, `function-calling` | `chat`, `function-calling` |
| **Tensor Parallel** | `0` | `0` (или `2` для fp16 на обоих GPU) |
| **Max Model Len** | `8192` | `8192` |
| **GPU Utilization** | `0.9` | `0.9` |

> [!IMPORTANT]
> **Отсутствующие параметры в лаунчере:**  
> Для корректной работы tool calling в vLLM необходимы параметры, которых может не быть в UI:
> - `--enable-auto-tool-choice`
> - `--tool-call-parser hermes`
> - `--reasoning-parser deepseek_r1` (только для Qwen3)
> 
> Если в лаунчере нет этих полей — их нужно добавить, иначе tool calling не будет работать.

---

## Пример конфигурации code-agents.yaml

```yaml
provider:
  base_url: "http://localhost:8000/v1"  # vLLM default port
  api_key: "none"

agents:
  planner:
    model:
      model: "Qwen/Qwen3-8B"
      temperature: 0.3
      max_tokens: 4096

  subplanner:
    model:
      model: "Qwen/Qwen3-8B"
      temperature: 0.3
      max_tokens: 4096

  worker:
    model:
      model: "zai-org/SWE-Dev-32B"
      temperature: 0.2
      max_tokens: 8192
```

---

## vLLM vs llama.cpp

| | vLLM | llama.cpp |
|------|------|-----------|
| **Формат** | HF (native), AWQ, GPTQ | GGUF |
| **Скорость** | Быстрее (PagedAttention, continuous batching) | Медленнее, но меньше VRAM |
| **Tool calling** | Нативная поддержка с парсерами | Через `--jinja` |
| **VRAM** | Больше (fp16 по умолчанию) | Меньше (GGUF квантизация) |
| **GPU** | Только CUDA | CUDA, Metal, CPU |
| **Tensor Parallel** | Да (multi-GPU из коробки) | Split-mode |
