# Markdown to HTML Converter

Create a simple Markdown to HTML converter in Go.

Requirements:
- Accept a .md file path as argument, output .html to stdout
- Support these Markdown elements:
  - Headers: `#`, `##`, `###` → `<h1>`, `<h2>`, `<h3>`
  - Bold: `**text**` → `<strong>text</strong>`
  - Italic: `*text*` → `<em>text</em>`
  - Code: `` `code` `` → `<code>code</code>`
  - Unordered lists: `- item` → `<ul><li>item</li></ul>`
  - Links: `[text](url)` → `<a href="url">text</a>`
  - Paragraphs: blank lines separate paragraphs
- Wrap output in a basic HTML template with `<html>`, `<head>`, `<body>`
- Add `--style` flag to include a simple embedded CSS

Constraints:
- Do NOT use any external markdown libraries — implement parsing yourself
- Single `main.go` file
- Include a sample test.md file for testing
- Commit the result with a clear message
