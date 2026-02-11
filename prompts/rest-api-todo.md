# REST API: Todo List

Create a simple REST API for a todo list in Go.

Requirements:
- Use standard `net/http` (no frameworks)
- In-memory storage (no database needed)
- Endpoints:
  - `GET /todos` — list all todos
  - `POST /todos` — create a new todo (JSON body: `{"title": "...", "done": false}`)
  - `GET /todos/{id}` — get a specific todo
  - `PUT /todos/{id}` — update a todo
  - `DELETE /todos/{id}` — delete a todo
- Return JSON responses with proper status codes
- Add a `GET /health` endpoint that returns `{"status": "ok"}`
- Listen on port 8080 (configurable via `PORT` env var)

Constraints:
- Keep it to 2-3 files max (main.go, handler.go, model.go)
- Add basic request logging middleware
- Commit the result with a clear message
