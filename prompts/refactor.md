# Refactor Task

Analyze the codebase in the current directory and identify areas that need refactoring.

Focus on:
- Code duplication: find repeated patterns and extract shared utilities
- Large functions: split functions longer than 50 lines into smaller pieces
- Naming: improve unclear variable/function names
- Error handling: ensure consistent error handling patterns
- Package boundaries: check for circular dependencies or misplaced code

Constraints:
- Do NOT change any public API signatures
- All existing tests must continue to pass
- Create new tests for any extracted functions
- Commit each logical refactoring step separately with clear messages
