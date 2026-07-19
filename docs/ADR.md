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
  this scope warrants. We keep a single port (`oracle.Source`) where an external boundary truly exists.

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
- Row locks (`FOR UPDATE`) around wallet/asset mutations in the service layer (wallets, listings,
  items, auctions, bids).

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

## ADR-009 — Auction settlement via background worker

**Decision.** A `SettlementWorker` ticker (`SETTLE_INTERVAL`, default 10s) calls
`AuctionService.SettleDue`, which settles each expired active auction in its own transaction
with the auction row locked (`FOR UPDATE`). With a highest bid, the winner's reservation is
converted to a spend (`SettleReserved`), the seller is credited, and the item transfers; with
no bids the item returns to Available.

**Why.** Auctions must close deterministically without a client request. Locking the auction row
and no-oping on already-settled auctions makes settlement **idempotent** and safe if several
workers (or ticks) run concurrently.

**Trade-offs.**
- Settlement latency is bounded by the tick interval, not instant at `ends_at`.
- A single-process ticker is not HA; multiple instances are safe (row lock) but do redundant scans.

**Alternatives considered.**
- **Settle lazily on read**: simpler, but an unread auction never closes and funds stay reserved.
- **Per-auction scheduled job/queue**: more precise timing, more infrastructure than the scope needs.

---

## ADR-010 — Oracle resilience: resilient client + last-known-good

**Decision.** The external feed is modelled as `oracle.Source`. A `ResilientClient` wraps it with
a per-attempt **timeout**, bounded **exponential-backoff retries**, and a **circuit breaker**
(fails fast with `ErrCircuitOpen` when tripped). `OracleService` **validates** every value
(reject zero/negative, above a plausibility ceiling, or large deviation), persists only accepted
prices to `oracle_prices`, and keeps an in-memory **last-known-good** cache warmed from the DB on
startup. A bad or missing feed never overwrites good data.

**Why.** The brief requires tolerating slow/wrong/zero/negative upstream responses without
corrupting state or crashing. Timeout+retry+breaker prevents cascading slowness; validation +
last-known-good preserves a usable price at all times.

**Trade-offs.**
- Last-known-good can be stale during a prolonged outage (no freshness SLA enforced).
- Breaker/validation thresholds are static config, not adaptive.

**Alternatives considered.**
- **Naive direct calls**: a slow upstream would block request paths; rejected.
- **Reject-and-fail on bad data**: violates the "stay reliable" requirement; rejected.

---

## ADR-011 — Idempotency via claim-and-replay

**Decision.** State-changing endpoints accept an `Idempotency-Key` header. The middleware
"claims" the key with a unique `INSERT ... ON CONFLICT DO NOTHING`; the winner runs the handler
and records the response for replay. A concurrent duplicate that loses the claim sees the
in-flight row and gets `409`; a completed key replays the stored response; a key reused with a
different request body is rejected with `409`.

**Why.** Retries and duplicate submissions must not cause a double effect (double buy / double
bid). The unique insert makes "at most one execution per key" a database guarantee, correct even
under concurrency.

**Trade-offs.**
- If the process dies after the business commit but before recording the response, the key stays
  in-flight; a TTL/sweeper would be needed for production (not implemented).
- Stored responses assume JSON; large bodies grow the table (no GC here).

**Alternatives considered.**
- **App-level de-dup map**: not durable, not multi-instance safe; rejected.
- **Natural idempotency only** (rely on DB unique constraints): covers asset uniqueness but not
  arbitrary duplicate POSTs; the key layer is more general.

---

## Known trade-off — daily purchase cap and auctions

The per-guild daily purchase cap is enforced on **limit-order buys**. It is intentionally **not**
re-checked at auction settlement: placing a bid reserves the funds (the real commitment), and the
winner simply pays from that reservation. Counting auction wins toward the daily cap would require
enforcing the cap at bid time against reserved funds. Documented here as a deliberate scope cut.

---

## What we'd add with more time

- Rich domain model / DDD-lite for the auction aggregate if the ruleset grows.
- Observability: metrics + tracing around money movements and settlement.
- Outbox pattern for domain events (`BidPlaced`, `AuctionSettled`) and async notifications.
- Authn/authz (guild identity) and rate limiting.
- Load/concurrency test suite proving no double-sale and no over-commit under contention.
- Read-model/caching for hot auction reads.
