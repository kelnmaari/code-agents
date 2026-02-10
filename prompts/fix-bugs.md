# Fix Bugs Task

Investigate and fix bugs in the codebase.

Approach:
1. Run existing tests to identify failures: `go test ./...`
2. Run the race detector: `go test -race ./...`
3. Run static analysis: `go vet ./...`
4. Read through recent git changes for suspicious patterns
5. Check error handling: look for ignored errors, missing nil checks
6. Check for resource leaks: unclosed files, goroutine leaks, missing defers
7. Check for data races: shared state without proper synchronization

For each bug found:
- Describe the bug and its root cause
- Write a failing test that reproduces it
- Fix the bug
- Verify the test passes
- Commit with a descriptive message

Constraints:
- Do NOT refactor unrelated code
- All existing tests must continue to pass
- Each bug fix should be a separate commit
- Include reproduction steps in commit messages
