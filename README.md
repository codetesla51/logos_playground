# Logos Playground Server

HTTP server for executing Logos language code in a sandboxed environment.

## Endpoints

- `POST /run` - Execute Logos code

## Rate Limiting

Uses token bucket algorithm (50 requests/10 seconds per IP)

## Running

```bash
go run main.go
```

Server starts on port 8080.
