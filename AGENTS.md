# Novro Engineering Guide

This file applies to the entire repository. More specific `AGENTS.md` files may
add rules for a subtree, but they must not weaken the security or verification
requirements below.

## Product Scope

- Build Novro as a modular monolith: one Go API service and one Next.js console.
- Keep domain boundaries explicit inside the monolith. Do not add network
  services merely to separate modules.
- The current milestone includes authentication, administrator user management,
  API keys, wallets, provider configuration, model routing, and the unified
  model gateway documented in the completed first-version roadmap.
- Do not implement payments, plans, subscriptions, coupons, organizations,
  projects, complex provider orchestration, multi-region deployment, or
  microservice decomposition until a later roadmap explicitly reaches them.

## Repository Layout

- `cmd/novro`: Go service entry point.
- `internal`: private Go application and domain packages.
- `ent`: Ent schemas and generated data access code.
- `ent/migrate`: ordered, reviewable database migrations.
- `apps/web`: Next.js console.
- `docs`: product, architecture, API, operations, and development records.

## Backend Rules

- Use the Go version declared in `go.mod` and keep `go.mod` tidy.
- Keep HTTP handlers thin. Put business invariants in application services and
  persistence details behind narrow repository interfaces where useful.
- Pass `context.Context` through request, service, and database boundaries.
- Wrap errors with actionable context, but never include passwords, tokens,
  connection strings, password hashes, or raw database errors in HTTP replies.
- Use structured logging. Do not log request authorization headers or cookies.
- Store passwords with a current adaptive password hash. Store session tokens
  only as cryptographic hashes.
- Enforce the last-active-administrator invariant transactionally.

## Frontend Rules

- Use TypeScript in strict mode and App Router conventions.
- Use unmodified shadcn/ui primitives as the component foundation. Local
  composition and styling are allowed; avoid replacing primitives with another
  UI kit.
- Support light and dark themes from the first screen. Every interactive
  control must be keyboard accessible and have an accessible name.
- Keep server-only configuration out of `NEXT_PUBLIC_*` variables.
- Display safe, user-oriented errors; never render internal API or database
  details.

## Configuration And Secrets

- Runtime configuration comes from environment variables. `.env.example` may
  contain names and safe defaults only.
- Never commit real passwords, API keys, session secrets, private keys, or
  production connection strings.
- The application must not run with a MySQL administrator account in normal
  operation. Use a least-privileged application account.
- Database TLS must remain enabled in production. The current cloud instance
  uses an untrusted certificate, so the MySQL client encrypts transport without
  certificate-chain verification and never falls back to plaintext.

## Database And Migrations

- Ent schemas are the application model source of truth.
- Every schema change requires an ordered SQL migration under `migrations`.
- Migrations must be reviewable, deterministic, forward-only in production, and
  safe to rerun through the migration runner.
- Do not call Ent automatic schema creation from the server at startup.
- Apply migrations explicitly before deployment and test them against MySQL.

## Verification

Run the narrowest relevant checks after each coherent change and the complete
suite before handoff:

```text
go test ./cmd/... ./internal/... ./ent/...
go vet ./...
pnpm --dir apps/web lint
pnpm --dir apps/web typecheck
pnpm --dir apps/web test
pnpm --dir apps/web build
```

Add or update tests for business invariants, authorization boundaries, config
validation, and HTTP behavior. A successful compile alone is not sufficient.

## Documentation And Git

- Update the relevant docs when behavior, configuration, API contracts, or
  operational steps change.
- Keep commits scoped to one verified stage. Do not commit generated output,
  local environment files, logs, coverage artifacts, or secrets.
- Preserve unrelated user changes and do not rewrite history unless explicitly
  requested.
