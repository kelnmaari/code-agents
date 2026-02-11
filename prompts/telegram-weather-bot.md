# Telegram Weather Bot

Create a simple Telegram bot in Go that shows weather for a given city.

Requirements:
- Use `github.com/go-telegram-bot-api/telegram-bot-api/v5` for Telegram API
- Use OpenWeatherMap free API (https://api.openweathermap.org/data/2.5/weather)
- Bot commands:
  - `/start` — welcome message with usage instructions
  - `/weather <city>` — show current weather (temperature, humidity, description)
- Read bot token and API key from environment variables: `TELEGRAM_BOT_TOKEN`, `OWM_API_KEY`
- Handle errors gracefully (invalid city, API failures)
- Create a `go.mod` file with proper module path

Constraints:
- Keep it simple: one `main.go` file is fine
- Add comments explaining key logic
- Commit the result with a clear message
