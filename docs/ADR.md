# Architecture Decision Record — Market Dragon

Status: living document. We update it as the build grows.

Context: this is a backend challenge. It is a marketplace for trading Common
and Rare items, and for auctioning Legendary items. It must be safe against
repeated requests, unreliable services, and many requests at the same time
(concurrency).

See `docs/IMPLEMENTATION_GUIDE.md` for the full plan.
See `docs/DESIGN_FA.md` for a Persian write-up of the trade-offs, the
double-spend guarantees, the business rules, and the Oracle handling.

Each decision below has four parts: **Decision**, **Why**, **Trade-offs**, and
**Alternatives considered**.

Note on terms used often here:
- A **row lock** stops two requests from changing the same database row at once.
- A **transaction** is a set of database changes that all pass or all fail.
- A **constraint** is a rule the database itself enforces on the data.

---

## ADR-001 — Layered architecture (not DDD, not hexagonal)

**Decision.** Use three layers: `api → service → repository`. Shared clients
live under `infra/`. Business rules live in `service/`. The `repository/` layer
holds simple data models with no behavior.

**Why.**
- It fits a short project. It is easy to read and to extend.
- The hardest promises (no double sale, no over-spending) are enforced by the
  database. So we do not need a heavy in-memory model to be correct.

**Trade-offs.**
- Models hold data but no logic. Logic sits in services. Over time, services can
  grow large.
- Rules are spread across small functions and database constraints. We need
  discipline to keep the rules in `service/`.

**Alternatives considered.**
- **DDD** (rich models, value objects, domain events): good for large, growing
  domains. It adds more code and mapping. Too much for this size.
- **Hexagonal / ports-and-adapters**: cleaner test seams, but more indirection
  than we need. We keep one port (`oracle.Source`) where a real external boundary
  exists.

---

## ADR-002 — PostgreSQL as the database

**Decision.** Use PostgreSQL 16.

**Why.**
- It has strong transactions. `SELECT ... FOR UPDATE` row locks and partial
  unique indexes let the database enforce our two key rules: one active auction
  per legendary, and a wallet that never goes negative. This holds even under
  concurrency.
- It is mature and easy to run with Docker Compose.

**Trade-offs.**
- Heavier than SQLite. It needs a running service (Compose handles this).
- A ledger works well in SQL. But very high-write bidding would need tuning
  (indexes, partitioning) that we did not do.

**Alternatives considered.**
- **SQLite**: no setup, but weak with many writers. A poor fit for concurrency.
- **NoSQL (Mongo/Redis)**: no easy multi-row ACID across wallet, item, and
  auction. It would push safety rules into app code, which we want to avoid.

---

## ADR-003 — GORM for database access

**Decision.** Use GORM with the PostgreSQL driver.

**Why.** It makes CRUD, transactions (`db.Transaction`), and row locks
(`clause.Locking{Strength: "UPDATE"}`) simple. It needs little code, which fits a
short project.

**Trade-offs.**
- The ORM can hide SQL cost. It has sharp edges. For example, a session that
  carries a finished statement must not be reused. We hit this while seeding and
  fixed it with fresh sessions.
- Less control than hand-written SQL for complex queries.

**Alternatives considered.**
- **sqlc** (typed SQL): more explicit and safer, but slower to iterate here.
- **database/sql + pgx directly**: most control, but more code.

---

## ADR-004 — golang-migrate with versioned SQL (not AutoMigrate)

**Decision.** The schema lives in versioned SQL files in `migrations/`
(`NNNN_create_<table>_table.up.sql` and `.down.sql`). They are embedded in the
binary and run on start and via `marketd migrate`.

**Why.**
- Schema changes are clear, reviewable, and reversible. Real SQL can express
  CHECK constraints and partial unique indexes that model tags cannot.
- It is predictable and close to production. A `schema_migrations` table tracks
  state.

**Trade-offs.**
- Two sources of truth: the SQL schema and the models. We keep model tags small,
  so the SQL is the source of truth.
- A bit more work than AutoMigrate.

**Alternatives considered.**
- **GORM AutoMigrate**: fast, but cannot express partial unique indexes and checks
  well. It is risky for destructive changes. Rejected.

---

## ADR-005 — Enforce the key rules in the database

**Decision.** Put the must-have rules into database constraints:
- Partial unique index `uniq_active_auction_per_item`: one active auction per item.
- CHECK `chk_wallet_nonneg`: `total ≥ 0 AND reserved ≥ 0 AND reserved ≤ total`.
- CHECK constraints for tiers and statuses, positive prices and bids, and
  non-negative stock and spending.
- Row locks (`FOR UPDATE`) around wallet and asset changes in the service layer
  (wallets, listings, items, auctions, bids).

**Why.** The database is the only place that stays correct under concurrency and
repeated requests. App-only checks can race and fail.

**Trade-offs.**
- Some rules live "in the schema". So reading the rules means reading SQL too.
- A broken constraint shows up as a database error. Services must turn it into a
  clear domain error.

**Alternatives considered.**
- **App-only checks**: easier to read, but unsafe under concurrency. Rejected.

---

## ADR-006 — Auction settlement with a background job

**Decision.** A settlement job runs on a timer (`SETTLE_INTERVAL`, default 10s).
It calls `AuctionService.SettleDue`. That method settles each expired auction in
its own transaction, with the auction row locked. If there is a top bid, the
winner pays from the reserved money, the seller gets paid, and the item moves.
If there are no bids, the item becomes available again.

**Why.** Auctions must close on their own, without a user request. Locking the
row and skipping already-closed auctions makes settlement safe to repeat. It is
also safe if more than one job runs at once.

**Trade-offs.**
- Close time depends on the timer, not the exact end time.
- One job process is not highly available. Many processes are safe (row lock),
  but they do repeated scans.

**Alternatives considered.**
- **Close on read**: simpler, but an item nobody reads never closes, and money
  stays reserved.
- **One scheduled job per auction**: better timing, but more infrastructure than
  we need.

---

## ADR-007 — Oracle resilience: safe client + last good price

**Decision.** The price feed is an interface, `oracle.Source`. A `ResilientClient`
wraps it with a timeout per call, a few retries with growing wait time (backoff),
and a circuit breaker. (A circuit breaker is a switch that stops calls to a
service that keeps failing.) When the breaker is open, calls fail fast with
`ErrCircuitOpen`.

`OracleService` checks every price. It rejects zero, negative, too-high, or
wildly changed values. It stores only good prices in `oracle_prices`. It keeps
the last good price in memory and loads it from the database at start. A bad or
missing feed never overwrites a good price.

**Why.** The task says the feed can be slow, wrong, zero, or negative. The system
must not crash or store bad data. Timeout, retry, and breaker stop slow calls
from spreading. Checks and the last good price keep a usable value at all times.

**Trade-offs.**
- The last good price can be old during a long outage. There is no freshness SLA.
- The breaker and check limits are fixed config, not adaptive.

**Alternatives considered.**
- **Plain direct calls**: a slow feed would block requests. Rejected.
- **Fail on bad data**: breaks the "stay reliable" rule. Rejected.

---

## ADR-008 — Idempotency with claim-and-replay

**Decision.** State-changing endpoints accept an `Idempotency-Key` header. The
middleware "claims" the key with a unique insert. The first caller runs the
handler and saves the response. A repeat caller replays the saved response. A
concurrent duplicate that is still running gets `409`. A key reused with a
different body also gets `409`.

**Why.** Retries and double clicks must not apply twice (no double buy or double
bid). The unique insert makes "at most one run per key" a database guarantee.
This is correct even under concurrency.

**Trade-offs.**
- If the process dies after the business change but before saving the response,
  the key stays "in progress". Production would need a cleanup job (not built).
- Saved responses are JSON. Large bodies grow the table (no cleanup here).

**Alternatives considered.**
- **In-memory de-dup map**: not durable, not safe across many instances. Rejected.
- **Rely only on unique constraints**: covers item uniqueness, but not any
  repeated POST. The key layer is more general.

---

## Known trade-off — daily limit and auctions

The daily spending limit applies to fixed-price buys. It is not re-checked at
auction settlement. A bid reserves the money, which is the real commitment. The
winner just pays from that reserve. To count auction wins in the daily limit, we
would need to check the limit at bid time against reserved money. This is a
deliberate scope cut.

---

## What we would add with more time

- A richer domain model for auctions, if the rules grow.
- Observability: metrics and tracing around money moves and settlement.
- An outbox pattern for events (`BidPlaced`, `AuctionSettled`) and async
  notifications.
- Login and access control, plus rate limiting.
- A load and concurrency test suite to prove no double sale and no over-spending
  under heavy use.
- A read model or cache for busy auction reads.
