# 04 — Go HTTP Client

Pure Go example showing how to call the MiniObserv API without any SDK. Uses only the standard library (`crypto/hmac`, `encoding/json`, `net/http`). A realistic reference for Go services that want to integrate with MiniObserv directly.

## What it does

1. Checks `/healthz`
2. Mints a HS256 JWT manually using `crypto/hmac` + `crypto/sha256`
3. Pushes `cpu.usage_pct` and `mem.used_pct` for host `go-example`
4. Queries `cpu.usage_pct` back for the last 5 minutes and prints the result

## Prerequisites

- Go >= 1.21
- A running MiniObserv server

## How to run

```sh
export MINIOBSERV_URL=http://localhost:8080
export AGENT_TOKEN=your-secret-here-min-16-chars
go run main.go
```

No `go.mod` needed — the file has no external dependencies.
