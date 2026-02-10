# Code Smell Report

## code-agents.yaml

### Quality Characteristics
- **Versioning**: The version is set to `1`.
- **Provider Configuration**: The provider is configured with a base URL and an API key.
- **Agent Configurations**:
  - **Planner**: Uses the model `qwen3-14b-gguf` with a temperature of `0.3` and a max token limit of `4096`.
  - **Subplanner**: Uses the model `qwen3-8b-gguf` with a temperature of `0.3` and a max token limit of `16384`.
  - **Worker**: Uses the model `qwen2-5-coder-14b-instruct-gguf` with a temperature of `0.2` and a max token limit of `40960`.
- **Orchestration Settings**:
  - **Loop**: Max depth is `3`, max workers is `4`, max steps is `100`, timeout is `30m`, and step delay is `2s`.
- **Tool Settings**:
  - **Work Directory**: Set to `.`.
  - **Allowed Shell**: Empty, allowing all shell commands.
  - **Git Enabled**: Set to `true`.
- **Input**:
  - **Prompt**: "Посмотри код и составь файл Code-smell.md с характеристикой качества кода".

### Recommendations
- **Security**: The API key should be stored securely and not hardcoded in the configuration file.
- **Versioning**: Consider using semantic versioning for better version management.
- **Documentation**: Add comments and documentation to the configuration file for better understanding.
- **Error Handling**: Implement error handling for file operations to handle cases where files do not exist.