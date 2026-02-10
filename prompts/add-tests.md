# Add Tests Task

Analyze the codebase and add comprehensive unit tests where coverage is lacking.

Focus on:
- Find packages and functions without test coverage
- Write table-driven tests following Go conventions
- Test edge cases: nil inputs, empty strings, boundary values, error paths
- Test concurrent access for any shared state (use -race flag)
- Mock external dependencies (HTTP clients, file system, etc.)

Constraints:
- Use the testify library for assertions (require for fatal, assert for non-fatal)
- Use t.TempDir() for any filesystem tests
- Use httptest.NewServer for HTTP tests
- Do NOT modify existing production code unless fixing a discovered bug
- Each test function should test one behavior
- Run `go test -race ./...` after adding tests to verify no races
