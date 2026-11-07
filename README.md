# Booking Systems API

Backend REST API (Go) for a booking system — portfolio project and a refresher on
Go backend architecture.

## Stack (current)

- **Router:** Gin, served through `net/http.Server` for manual graceful shutdown.
- **ORM:** GORM, connecting to a Supabase-hosted Postgres database.
- **Config:** environment variables (`.env` locally, see `.env.example`).
- **Logging:** structured logging via `log/slog`.

## Project layout

```
cmd/api/              # composition root — wires config, db, router, server
internal/
  health/             # liveness/readiness endpoints
  platform/
    config/           # env-based configuration
    database/         # GORM/Postgres connection
    httpserver/       # Gin router + graceful-shutdown HTTP server
```

## Running locally

1. Copy `.env.example` to `.env` and fill in your Supabase `DATABASE_URL`.
2. `make run`

## Endpoints

- `GET /health/live` — process liveness, no dependencies checked.
- `GET /health/ready` — liveness + database connectivity check.

## Notes

- `PrepareStmt` is disabled in GORM because Supabase's PgBouncer pooler
  (transaction mode) doesn't support prepared statements across requests.
- This diverges from the `sqlc` + `chi`/`net/http` stack originally sketched in
  `CLAUDE.md` — GORM and Gin were chosen for this iteration instead.
