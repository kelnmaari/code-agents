# CLI Calculator

Create a simple command-line calculator in Go.

Requirements:
- Read mathematical expressions from stdin (interactive mode)
- Support operations: `+`, `-`, `*`, `/`
- Support parentheses for grouping: `(2 + 3) * 4`
- Support floating point numbers
- Print the result after each expression
- Type `exit` or `quit` to close
- Handle errors: division by zero, invalid expressions

Example session:
```
> 2 + 3
= 5
> (10 - 2) * 3
= 24
> 100 / 0
Error: division by zero
> exit
Goodbye!
```

Constraints:
- Single `main.go` file
- Do NOT use `eval` or shell commands — implement a proper expression parser
- Commit the result with a clear message
