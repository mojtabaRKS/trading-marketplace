# Architecture Decision Record — Market Dragon

Status: living document (updated as the build progresses).
Context: Backend challenge — a secure marketplace for trading (Common/Rare) and
auctioning (Legendary) items, resilient to duplicates, unreliable services, and
concurrency. See `docs/IMPLEMENTATION_GUIDE.md` for the full brief.

Each decision below lists **Decision**, **Why**, **Trade-offs**, and **Alternatives considered**.

---

## ADR-001 — Layered / service-oriented architecture (not DDD, not hexagonal)

**Decision.** Organize as `api → service → repository` with cross-cutting clients under `infra/`.
Business rules live in `service/`; `repository/` holds anemic GORM models.

**Why.**
- Fits the 1–3 day timebox; low ceremony, fast to read and extend.
- The hardest guarantees (unique asset sale, wallet solvency) are enforced at the
  **database** layer, so a rich in-memory domain model is not required for correctness.

**Trade-offs.**
- Models are anemic (data, no behavior) — logic sits in services, which can grow into
  "transaction scripts" as features accumulate.
- Business invariants are not centralized on an aggregate; they are spread across pure
  rule functions + DB constraints. Requires discipline to keep rules in `service/`.

**Alternatives considered.**
- **DDD** (rich aggregates like `Auction.PlaceBid`, value objects `Money`, domain events):
  best for large, evolving domains; adds a domain package and model↔entity mapping. Overkill here.
- **Hexagonal/ports-and-adapters**: cleaner test seams via ports, but more indirection than
  this scope warrants. We keep a single port (`oracle.Port`) where an external boundary truly exists.

---

## ADR-002 — PostgreSQL as the datastore

**Decision.** PostgreSQL 16.

**Why.**
- Strong transactional guarantees: `SELECT ... FOR UPDATE` row locks and partial unique
  indexes let the DB enforce the two critical invariants (one active auction per legendary,
  wallet non-negative/solvent) even under concurrent requests.
- Mature, ubiquitous, easy to run via Docker Compose.

**Trade-offs.**
- Heavier than SQLite for a challenge; needs a running service (handled by Compose).
- Relational modeling of a ledger is fine, but high-write auction bidding would eventually
  need tuning (indexes, partitioning) not done here.

**Alternatives considered.**
- **SQLite**: zero-ops, but weaker concurrency story (single writer) — a poor fit for the
  concurrency requirement.
- **NoSQL (Mongo/Redis)**: no native multi-row ACID across wallet+item+auction; would push
  invariant enforcement into app code, exactly what we want to avoid.

---

## ADR-003 — GORM for data access

**Decision.** GORM with the pgx/postgres driver.

**Why.** Fast CRUD, transactions (`db.Transaction`), and locking clauses
(`clause.Locking{Strength: "UPDATE"}`) are first-class; low boilerplate for the timebox.

**Trade-offs.**
- ORM abstraction can hide SQL cost and has sharp edges (e.g. sessions carrying a finished
  statement must not be reused — hit during seeding, fixed with fresh sessions).
- Less control than hand-written SQL for complex queries.

**Alternatives considered.**
- **sqlc** (typed SQL): more explicit and safer, but slower to iterate for this scope.
- **database/sql + pgx directly**: maximal control, more boilerplate.

---

## ADR-004 — golang-migrate with versioned SQL (not GORM AutoMigrate)

**Decision.** Schema owned by versioned SQL files in `migrations/`
(`NNNN_create_<table>_table.up.sql`/`.down.sql`), embedded via `//go:embed` and applied on
startup and via `marketd migrate`.

**Why.**
- Explicit, reviewable, reversible schema changes with real DDL (CHECK constraints, partial
  unique indexes) that GORM tags cannot fully express.
- Deterministic and production-realistic; `schema_migrations` tracks state.

**Trade-offs.**
- Two sources of truth to keep aligned: SQL schema and GORM models. Mitigated by keeping
  model tags minimal (schema is authoritative).
- Slightly more upfront work than `AutoMigrate`.

**Alternatives considered.**
- **GORM AutoMigrate**: quick, but can't express partial unique indexes/checks cleanly and
  is risky for destructive changes. Rejected.

---

## ADR-005 — Enforce invariants at the database layer

**Decision.** Encode the non-negotiables as DB constraints:
- Partial unique index `uniq_active_auction_per_item` → one active auction per item.
- CHECK `chk_wallet_nonneg` → `total ≥ 0 AND reserved ≥ 0 AND reserved ≤ total`.
- CHECK constraints for tiers/statuses, positive prices/bids, non-negative stock/spend.
- Row locks (`FOR UPDATE`) around wallet/asset mutations (in the service layer, upcoming steps).

**Why.** The database is the only place that is correct under concurrency and duplicate
requests. App-level checks alone race.

**Trade-offs.**
- Some business logic lives "in the schema", so reading rules means reading SQL too.
- Constraint violations surface as DB errors that services must translate to domain errors.

**Alternatives considered.**
- **App-only enforcement**: simpler to read but unsafe under concurrency. Rejected.

---

## ADR-006 — Cobra CLI + Viper configuration

**Decision.** `cobra` root `marketd` with `serve`/`migrate`/`seed`; `viper` loads a `.env`
file then environment variables (precedence: env > .env > default).

**Why.** Separates operational concerns (run server vs. migrate vs. seed) and gives flexible,
12-factor-friendly config with a good local-dev experience.

**Trade-offs.**
- More dependencies and a little wiring vs. a bare `main` + `envconfig`.
- Viper is a large dependency for a small config surface.

**Alternatives considered.**
- **Flag/stdlib + envconfig** (previous approach): lighter, but no subcommands and weaker
  `.env` ergonomics.

---

## ADR-007 — Money as integer minor units

**Decision.** All monetary values are `int64` minor units; never floats.

**Why.** Avoids floating-point rounding errors in balances, reserves, and the +5% bid rule.

**Trade-offs.** Callers must agree on the unit scale; no fractional sub-unit precision.

---

## ADR-008 — Local DB port 5433

**Decision.** Compose maps Postgres to host `5433:5432`.

**Why.** The developer's host already runs another Postgres on `5432`; `5433` avoids the
conflict. Inside the Compose network the app still reaches `db:5432`.

**Trade-offs.** Local tooling must target `5433` (documented in `.env.example`/Makefile).

---

## Planned decisions (to be recorded as implemented)

- **Idempotency**: `Idempotency-Key` header + `idempotency_keys` table storing replayed
  responses for state-changing endpoints.
- **Oracle resilience**: `oracle.Port` behind timeout + retry/backoff + circuit breaker;
  validate prices (reject zero/negative), keep last-known-good.
- **Auction settlement**: background worker with per-auction row lock for exactly-once settlement.

---

## What we'd add with more time

- Rich domain model / DDD-lite for the auction aggregate if the ruleset grows.
- Observability: metrics + tracing around money movements and settlement.
- Outbox pattern for domain events (`BidPlaced`, `AuctionSettled`) and async notifications.
- Authn/authz (guild identity) and rate limiting.
- Load/concurrency test suite proving no double-sale and no over-commit under contention.
- Read-model/caching for hot auction reads.
