# File Organizer

Create a CLI tool in Go that organizes files in a directory by their extension.

Requirements:
- Accept a source directory as a CLI argument
- Group files by extension into subdirectories:
  - `images/` — .jpg, .jpeg, .png, .gif, .svg, .webp
  - `documents/` — .pdf, .doc, .docx, .txt, .md
  - `code/` — .go, .py, .js, .ts, .html, .css
  - `archives/` — .zip, .tar, .gz, .rar, .7z
  - `other/` — everything else
- Move files (not copy) into the appropriate subdirectory
- Print a summary: how many files moved to each category
- Add `--dry-run` flag that shows what WOULD happen without moving anything
- Add `--undo` flag that reads a log file and reverts the last organization

Constraints:
- Use `github.com/alexflint/go-arg` for CLI parsing
- Create a proper go.mod
- Handle edge cases: empty directory, duplicate filenames, hidden files
- Commit the result with a clear message
