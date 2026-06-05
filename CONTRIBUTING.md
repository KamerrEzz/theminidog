# Contributing

Thanks for your interest in contributing to MiniObserv.

## Getting started

```bash
git clone https://github.com/KamerrEzz/theminidog.git
cd theminidog
go test ./...   # all tests must pass
```

## How to contribute

- **Bug reports** → open an issue using the Bug Report template
- **Feature ideas** → open an issue to discuss before submitting a PR
- **PRs** → keep them focused (one change per PR), include tests, follow existing patterns

## Rules

- All new Go code must have tests. Run `go test ./...` before submitting.
- Follow the existing package structure and naming conventions.
- No external dependencies without discussion — Weeks 4 and 5 were built with stdlib only.
- Commit messages follow [Conventional Commits](https://conventionalcommits.org).
- No AI attribution in commits or code (see CLAUDE.md philosophy).

## Running locally

```bash
# Server (needs TimescaleDB)
DATABASE_URL=postgres://minidog:minidog@localhost:5432/miniobserv \
AGENT_TOKEN=dev-secret \
go run ./cmd/server

# Agent
SERVER_URL=http://localhost:8080 \
AGENT_TOKEN=dev-secret \
go run ./cmd/agent

# Full stack
cd deployments && docker compose up --build
```

## Questions?

Open an issue or join the community at [zeew.space/discord](https://zeew.space/discord).
