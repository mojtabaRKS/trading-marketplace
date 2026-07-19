# Market Dragon — Implementation Guide

> Step-by-step build plan for the **Market Dragon** backend challenge (Herotech v1.0.1).
> Each numbered **Step** below is self-contained and written so it can be pasted as a future prompt.
> The original brief lives in `.vscode/desc.txt` (Persian). This file is the English, actionable translation of it.

---

## 0. What we are building (context for every prompt)

Design and implement the core of a **secure marketplace** for buying and auctioning
**legendary items** in the land of **Aethoria**. The market guardian, **Vorynthax**, has
three non-negotiable expectations:

1. **No trade may cause an invalid or duplicate sale of an asset.** (Legendary items are unique.)
2. **No Guild may commit to a purchase beyond its available funds or its daily purchase quota.**
3. **The market must stay reliable and auditable even when external services misbehave.**

The system must be **defensible against**:
- Duplicate operations (idempotency)
- Unreliable external services (timeouts / bad data)
- Concurrent requests (race conditions on unique assets and wallets)

### Domain model summary

**Items** — three tiers:
| Tier | Stock | Reproducible? | Trade mechanism |
|------|-------|---------------|-----------------|
| Common | High | Yes | Limit Order |
| Rare | Limited / fixed | Yes, but slow | Limit Order |
| Legendary | Exactly one in the world | Never | Auction only |

Example legendaries: Sword **Soul Reaver**, Ring **Eye of the Dragon**.

**Guilds** — each Guild:
- Has a gold **wallet**.
- Has a **daily purchase cap** (anti-monopoly).
- Can **list** its items for sale.
- Can **bid** on items.

**Wallet** — `Available Balance = Total Balance − Reserved Amount`.
Every movement must be traceable: **purchase**, **reserve**, **release**.

### Mechanisms

**Limit Order** (Common & Rare):
- Seller lists item at a fixed price.
- If buyer has enough available balance, buyer buys at that price.
- Price is frozen once the order is placed.

**Auction** (Legendary only):
- Each Legendary can have **only one active auction** at a time.
- Auction has a time window (default **24h**, configurable).
- Highest bid wins.
- **Anti-sniping:** a bid in the last **5 minutes** extends the auction by **5 minutes**.
- Placing a bid **reserves** the amount (money is NOT deducted).
- When a bid is no longer winning, its reserved amount is **released immediately**.
- If the auction ends with **no bids**, the item becomes **Available** again.

### Business rules (must be enforced)
1. A Guild cannot bid on its own item.
2. A new bid must be **≥ 5% higher** than the current highest bid.
3. A bid can be cancelled **only if you are not the current highest bidder**.
4. Before any financial commitment, balance (including reserves) is checked.
5. Each Legendary has at most one active auction at any moment.

### External service
**Oracle Price Service** — pushes base item prices every ~30s. It may be slow,
return wrong prices, or send **zero/negative** values. The system must tolerate all of this.

### Technical constraints
- **Language:** Go.
- **Database:** free choice — but the choice must be justified (see ADR).
- **External services:** must be **mocked** behind a defined **interface**.
- **Tests:** optional but expected for sensitive logic.

### Deliverables (definition of done)
1. Source code + complete **README** in a public repo.
2. **Run instructions** (Docker Compose or similar).
3. An **ADR** (Architecture Decision Record): why this architecture, trade-offs made,
   and what you'd add with more time.

---

## Recommended tech stack (default decisions)

These are sensible defaults; revisit in the ADR.

- **Language:** Go 1.26
- **Database:** PostgreSQL — chosen for strong transactional guarantees (row-level locking,
  `SERIALIZABLE`/`SELECT ... FOR UPDATE`), which the unique-asset and wallet invariants demand.
- **HTTP:** `Gin` framework.
- **CLI:** `cobra` (root `marketd` + `serve`/`migrate`/`seed` subcommands).
- **Config:** `viper` (loads `.env` + env vars; env > .env > default).
- **DB access / ORM:** `GORM` with the Postgres driver.
- **Migrations:** `golang-migrate` over versioned SQL files in `migrations/` (one file per table),
  embedded with `//go:embed` and applied on startup.
- **Config:** env vars via `envconfig`/`viper`.
- **Logging:** `slog` (stdlib, structured).
- **Testing:** stdlib `testing` + `testcontainers-go` for integration tests.
- **Containerization:** Docker + Docker Compose (app + Postgres + mock oracle).
- **Auction expiry:** background worker (ticker) that settles expired auctions.

---

## Architecture (layered / hexagonal-lite)

```
cmd/marketd/            # cobra CLI: root + serve/migrate/seed
migrations/             # golang-migrate SQL, one file per table + embed.FS
internal/
  config/               # viper config (DSN + DatabaseURL)
  api/                  # HTTP: Gin router + server lifecycle
    middleware/         # all Gin middlewares (logging, recovery, idempotency)
  service/              # use-cases; owns DB transaction boundaries
  repository/           # persistence models + data-access repositories
  infra/
    database/           # GORM client (Open) + migration runner (Migrate)
    logging/            # slog logger builder
    oracle/             # oracle port + client/mock
  worker/               # auction settlement ticker
docker-compose.yml
Dockerfile
README.md
docs/ADR.md
```

Flow is `api → service → repository`. Models live in `repository/`. Put **transaction boundaries**
in the service layer. Enforce concurrency/uniqueness invariants at the **database** level
(constraints + locks), never solely in Go memory.

---

## Implementation steps (use each as a future prompt)

Each step lists **Goal**, **Tasks**, and **Acceptance criteria**. Do them in order.

### Step 1 — Project scaffolding & tooling
**Goal:** A runnable, empty-but-wired Go service.
**Tasks:**
- `go mod init`, set Go 1.22+, add chosen deps.
- Create the folder layout above.
- Add `Dockerfile`, `docker-compose.yml` (app + Postgres), `Makefile` (build/run/test/migrate).
- Add config loading from env; a `/healthz` endpoint.
- Add structured logging and graceful shutdown.
**Acceptance:** `docker compose up` starts the app + DB; `GET /healthz` returns 200.

### Step 2 — Data model & migrations
**Goal:** Schema that encodes the invariants.
**Tasks:**
- Tables: `guilds`, `wallets`, `wallet_transactions`, `items`, `listings` (limit orders),
  `auctions`, `bids`, `oracle_prices`, `idempotency_keys`, `daily_purchase_totals`.
- Encode constraints: unique legendary item, **partial unique index** ensuring at most one
  `active` auction per item, non-negative balances, enum-like check constraints for item tier
  and statuses.
- Seed script with a few guilds, common/rare/legendary items.
**Acceptance:** Migrations apply cleanly; seed data loads; constraints reject duplicate active auctions.

### Step 3 — Domain layer (pure logic + unit tests)
**Goal:** Business rules independent of DB/HTTP.
**Tasks:**
- Value objects: `Money` (integer minor units), item tiers, statuses.
- Wallet logic: `Available = Total − Reserved`; reserve/release/debit with guards.
- Bid rules: min +5% increment, no self-bid, cannot cancel while highest bidder.
- Auction rules: single active auction, anti-snipe extension (5 min), winner selection.
- Daily cap check helper.
**Acceptance:** Unit tests cover each business rule (happy + rejection paths). No I/O in domain.

### Step 4 — Wallet service with traceable transactions
**Goal:** Reliable, auditable money movements.
**Tasks:**
- Implement `Reserve`, `Release`, `Debit` (purchase) as DB transactions.
- Every movement writes a `wallet_transactions` row (type, amount, ref, timestamp).
- Use `SELECT ... FOR UPDATE` on the wallet row to serialize concurrent changes.
- Enforce `available >= amount` before reserve/debit.
**Acceptance:** Concurrent reserves on the same wallet never over-commit funds; ledger reconciles
(`sum(transactions) == wallet balances`).

### Step 5 — Limit Order flow (Common & Rare)
**Goal:** List and buy at a fixed price.
**Tasks:**
- `POST /listings` (seller lists item + fixed price).
- `POST /listings/{id}/buy` (buyer purchases): check available balance, check daily cap,
  debit buyer, credit seller, transfer item ownership, mark listing sold — all in one TX.
- Prevent double-sale: lock listing row / status transition `open -> sold` atomically.
- Rare items: respect limited stock.
**Acceptance:** An item can be sold once; concurrent buys → exactly one succeeds; daily cap enforced.

### Step 6 — Auction lifecycle (Legendary)
**Goal:** Create and run auctions.
**Tasks:**
- `POST /auctions` (start auction for a legendary; enforce single active auction, 24h window).
- `POST /auctions/{id}/bids`: validate not-self, +5% rule, reserve funds, release previous
  highest bidder's reserve, apply anti-snipe extension.
- `DELETE /auctions/{id}/bids/{bidId}`: allowed only if not current highest; release reserve.
- Read endpoints: auction detail, current highest bid, bid history.
**Acceptance:** Bids enforce all rules; reserves move correctly between bidders; extension works.

### Step 7 — Auction settlement worker
**Goal:** Close auctions deterministically.
**Tasks:**
- Background ticker scans for expired active auctions.
- Winner: debit reserved amount → transfer item → mark won; release all losers.
- No bids: release nothing to release, mark item **Available** again.
- Make settlement **idempotent** and safe under concurrent workers (lock the auction row).
**Acceptance:** Expired auctions settle exactly once; winner gets item; funds/ownership consistent.

### Step 8 — Oracle Price Service (interface + mock + resilience)
**Goal:** Consume an unreliable external price feed safely.
**Tasks:**
- Define `OraclePort` interface; implement a **mock** that can be slow / wrong / zero / negative.
- Poll every ~30s with **timeout**, **retries/backoff**, and a **circuit breaker**.
- **Validate** prices: reject zero/negative/implausible; keep last-known-good.
- Store accepted prices in `oracle_prices`; expose current base price.
**Acceptance:** Bad/slow oracle responses never corrupt state or crash the service; last-good used.

### Step 9 — Idempotency & concurrency hardening
**Goal:** Survive duplicates and races (Vorynthax's demand).
**Tasks:**
- `Idempotency-Key` header on all state-changing endpoints; store + replay results.
- Confirm DB-level uniqueness for unique-asset sale and single active auction.
- Add targeted concurrency tests (parallel buys, parallel bids, duplicate requests).
**Acceptance:** Replaying a request has no double effect; race tests pass under `-race`.

### Step 10 — Tests, docs & delivery
**Goal:** Meet the deliverables.
**Tasks:**
- Integration tests (testcontainers) for the critical flows (buy, auction, settlement, oracle).
- Complete **README**: overview, setup, run (Docker Compose), API examples, assumptions.
- Write **docs/ADR.md**: DB choice rationale, concurrency/idempotency strategy, trade-offs,
  and "what I'd add with more time".
- Ensure `docker compose up` runs the whole system end-to-end.
**Acceptance:** Fresh clone → `docker compose up` → all documented flows work; tests green.

---

## Cross-cutting checklist (verify before submitting)
- [ ] Legendary uniqueness enforced at DB level (no duplicate sale possible).
- [ ] Wallet never goes negative; `Available = Total − Reserved` always holds.
- [ ] Daily purchase cap enforced per guild.
- [ ] +5% bid rule, no self-bid, cancel-only-if-not-highest.
- [ ] One active auction per legendary (partial unique index).
- [ ] Anti-snipe 5-min extension works.
- [ ] All money movements recorded in ledger and reconcile.
- [ ] Oracle failures (slow/zero/negative/wrong) handled gracefully.
- [ ] Idempotent state-changing endpoints.
- [ ] Passes `go test -race ./...`.
- [ ] README + Docker Compose + ADR present.

---

## Submission (from the brief)
- Email: `arash.sadeghian.haghighi@gmail.com`
- Subject: `چالش فنی Golang – نام و نام خانوادگی` (Golang challenge – Full Name)
- Phone/WhatsApp: `09190058697` — https://wa.me/+989190058697
- Suggested effort: **1–3 working days** (ask if you need more).
