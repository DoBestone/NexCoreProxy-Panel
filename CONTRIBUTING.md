## Local Development Setup

1. Clone the repository
2. Create a directory named `x-ui` in the project root
3. Copy `.env.example` to `.env` and configure
4. Run `go run main.go`

## Project Structure

- `main.go` — Application entry point
- `web/` — Web UI and HTTP server (based on 3X-UI)
- `ncp/` — NexCoreProxy Agent and API integration
- `config/` — Configuration management
- `database/` — Database models and migrations
- `xray/` — Xray-core process management

## Code Style

- Run `gofmt` before committing
- Pass `go vet` and `staticcheck` checks
- Follow existing code patterns
