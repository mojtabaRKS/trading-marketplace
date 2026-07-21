# Market Dragon — Implementation Guide

> This is the step-by-step build plan for the Market Dragon backend challenge
> (Herotech v1.0.1). Each step stands on its own.
> The original brief is in `.vscode/desc.txt` (Persian). This file is the English
> version you can act on.

---

## 0. What we are building

We build the core of a secure marketplace. Guilds buy and sell items in the land
of Aethoria, and auction Legendary items. The market guardian, Vorynthax, has
three rules we must not break:

1. No trade may sell an item that does not exist, or sell it twice.
   (Legendary items are unique.)
2. No guild may spend more than its available money or its daily limit.
3. The market must stay reliable and easy to audit, even when outside services
   fail. ("Audit" means we can trace every action later.)

The system must be safe against:
- Repeated requests (idempotency).
- Unreliable outside services (timeouts and bad data).
- Many requests at the same time (races on unique items and wallets).

### Domain model

**Items** — three tiers:

| Tier | Stock | Can be remade? | How it is traded |
|------|-------|---------------|-----------------|
| Common | High | Yes | Limit Order |
| Rare | Limited | Yes, but slowly | Limit Order |
| Legendary | Only one in the world | Never | Auction only |

Example legendaries: the sword **Soul Reaver** and the ring **Eye of the Dragon**.

**Guilds** — each guild:
- Has a gold wallet.
- Has a daily spending limit (to stop one guild from buying everything).
- Can list its items for sale.
- Can bid on items.

**Wallet** — `Available Balance = Total Balance − Reserved Amount`.
Every money move must be traceable: purchase, reserve, release.

### Mechanisms

**Limit Order** (Common and Rare):
- The seller lists an item at a fixed price.
- A buyer with enough available money buys it at that price.
- The price does not change once the order is placed.

**Auction** (Legendary only):
- Each Legendary can have only one active auction at a time.
- An auction has a time window (default 24h, configurable).
- The highest bid wins.
- Anti-snipe: a bid in the last 5 minutes adds 5 more minutes.
- A bid reserves the money. The money is not taken yet.
- When a bid is no longer winning, its reserved money is freed right away.
- If the auction ends with no bids, the item becomes available again.

### Business rules (must be enforced)
1. A guild cannot bid on its own item.
2. A new bid must be at least 5% higher than the current top bid.
3. A bid can be cancelled only if you are not the top bidder.
4. Before any money is committed, the balance (including reserves) is checked.
5. Each Legendary has at most one active auction at any time.

### Outside service
The **Oracle Price Service** sends base item prices about every 30 seconds. It
may be slow, send wrong prices, or send zero or negative values. The system must
handle all of this.

### Technical rules
- Language: Go.
- Database: free choice, but the choice must be explained (see ADR).
- Outside services: must be mocked behind a clear interface.
- Tests: optional, but expected for sensitive logic.

### Deliverables (definition of done)
1. Source code and a full README in a public repo.
2. Run instructions (Docker Compose or similar).
3. An ADR (Architecture Decision Record): why this design, the trade-offs, and
   what you would add with more time.

---

## Recommended tech stack

These are good defaults. We explain them in the ADR.

- **Language:** Go 1.26
- **Database:** PostgreSQL — chosen for strong transactions (row locks,
  `SELECT ... FOR UPDATE`), which the unique-item and wallet rules need.
- **HTTP:** Gin.
- **CLI:** Cobra (root `marketd` plus `serve`, `migrate`, `seed`).
- **Config:** Viper (loads `.env` and env vars; env > .env > default).
- **Database access:** GORM with the PostgreSQL driver.
- **Migrations:** golang-migrate over versioned SQL files in `migrations/`,
  embedded with `//go:embed` and run on start.
- **Logging:** slog (standard library, structured logs).
- **Testing:** standard `testing`, plus a running PostgreSQL for integration tests.
- **Containers:** Docker and Docker Compose (app plus PostgreSQL).
- **Auction close:** a background job (timer) that settles ended auctions.

---

## Architecture

```
cmd/marketd/            # CLI: serve / migrate / seed
migrations/             # golang-migrate SQL, one file per table + embed.FS
internal/
  config/               # viper config (DSN + DatabaseURL)
  api/                  # HTTP: Gin router + server start/stop
    middleware/         # Gin middlewares (logging, recovery, idempotency)
  service/              # use-cases; owns queries, writes, and transactions
  model/                # data models + seed data (no data access logic)
  infra/
    database/           # GORM client (Open) + migration runner (Migrate)
    logging/            # slog logger setup
    oracle/             # oracle interface + client/mock
  worker/               # auction settlement job
docker-compose.yml
Dockerfile
README.md
docs/ADR.md
```

The flow is `api → service → model`. Models live in `model/`, which
holds no data-access logic. The service layer owns all queries, writes, and
database transactions. Enforce concurrency and uniqueness in the database
(constraints and locks), not only in Go memory.

---

## Implementation steps

Each step lists **Goal**, **Tasks**, and **Acceptance**. Do them in order.

### Step 1 — Project setup and tooling
**Goal:** A Go service that runs, even if empty.
**Tasks:**
- `go mod init`, set Go 1.22+, add the chosen libraries.
- Create the folder layout above.
- Add `Dockerfile`, `docker-compose.yml` (app + PostgreSQL), and a `Makefile`
  (build, run, test, migrate).
- Load config from env. Add a `/health` endpoint.
- Add structured logging and a clean shutdown.
**Acceptance:** `docker compose up` starts the app and the database.
`GET /health` returns 200.

### Step 2 — Data model and migrations
**Goal:** A schema that holds the rules.
**Tasks:**
- Tables: `guilds`, `wallets`, `wallet_transactions`, `items`, `listings`,
  `auctions`, `bids`, `oracle_prices`, `idempotency_keys`, `daily_purchase_totals`.
- Add constraints: unique legendary item; a partial unique index for one active
  auction per item; non-negative balances; check constraints for tiers and
  statuses.
- Add a seed script with a few guilds and some common, rare, and legendary items.
**Acceptance:** Migrations run cleanly. Seed data loads. Constraints reject a
second active auction.

### Step 3 — Service layer: business rules (with unit tests)
> Note: this project is layered (not DDD). So the "domain" logic lives in
> `internal/service/` as small, pure functions, not in a separate `domain/`
> package. See `docs/ADR.md` (ADR-001) for the layered-vs-DDD trade-off.

**Goal:** Write the rules as pure functions in `internal/service/`, with no
database or HTTP.
**Tasks:**
- Helpers (integer minor units, no floats): `AvailableBalance = Total − Reserved`,
  plus reserve and balance guards.
- Bid rules: +5% minimum, no self-bid, no cancel while top bidder.
- Auction rules: anti-snipe extension (5 min), and an auction-ended check.
- Daily limit helper (limit 0 = unlimited).
- Named errors (`ErrInsufficientFunds`, `ErrSelfBid`, `ErrBidTooLow`, ...).
**Acceptance:** Unit tests cover each rule (pass and fail cases). No I/O in the
rule code.

### Step 4 — Wallet service with a full history
**Goal:** Reliable, traceable money moves.
**Tasks:**
- Add `Reserve`, `Release`, and `Debit` as database transactions.
- Every move writes a `wallet_transactions` row (type, amount, ref, time).
- Use `SELECT ... FOR UPDATE` on the wallet row to serialize changes.
- Check `available >= amount` before reserve or debit.
**Acceptance:** Concurrent reserves on one wallet never over-spend. The ledger
matches the balance (`sum(transactions) == wallet balances`).

### Step 5 — Limit Order flow (Common and Rare)
**Goal:** List and buy at a fixed price.
**Tasks:**
- `POST /listings` (seller lists an item at a fixed price).
- `POST /listings/{id}/buy` (buyer buys): check the balance, check the daily
  limit, debit the buyer, pay the seller, move the item, mark the listing sold —
  all in one transaction.
- Stop double sales: lock the listing row and change `open -> sold` atomically.
- Rare items: respect the limited stock.
**Acceptance:** An item sells once. Concurrent buys → exactly one wins. The daily
limit is enforced.

### Step 6 — Auction lifecycle (Legendary)
**Goal:** Create and run auctions.
**Tasks:**
- `POST /auctions` (start an auction for a legendary; one active auction per item,
  24h window).
- `POST /auctions/{id}/bids`: check not-self, apply +5%, reserve money, free the
  previous top bidder, and apply anti-snipe.
- `DELETE /auctions/{id}/bids/{bidId}`: allowed only if not the top bidder; free
  the reserve.
- Read endpoints: auction detail, top bid, and bid history.
**Acceptance:** Bids follow all rules. Reserves move between bidders correctly.
The extension works.

### Step 7 — Auction settlement job
**Goal:** Close auctions in a clear, repeatable way.
**Tasks:**
- A background timer finds ended active auctions.
- Winner: take the reserved money → move the item → mark it won.
- No bids: nothing to free; mark the item available again.
- Make settlement safe to repeat and safe with many jobs (lock the auction row).
**Acceptance:** Ended auctions settle once. The winner gets the item. Money and
ownership stay correct.

### Step 8 — Oracle Price Service (interface + mock + resilience)
**Goal:** Use an unreliable price feed safely.
**Tasks:**
- Define the `oracle.Source` interface. Write a mock that can be slow, wrong,
  zero, or negative.
- Poll about every 30s with a timeout, retries/backoff, and a circuit breaker.
- Check prices: reject zero, negative, and impossible values; keep the last good
  price.
- Store good prices in `oracle_prices`. Expose the current base price.
**Acceptance:** Bad or slow oracle answers never corrupt state or crash the
service. The last good price is used.

### Step 9 — Idempotency and concurrency hardening
**Goal:** Survive repeats and races.
**Tasks:**
- `Idempotency-Key` header on all state-changing endpoints; store and replay
  results.
- Confirm the database uniqueness for the unique-item sale and the single active
  auction.
- Add focused concurrency tests (parallel buys, parallel bids, repeated requests).
**Acceptance:** Replaying a request has no double effect. Race tests pass with the
race detector.

### Step 10 — Tests, docs, and delivery
**Goal:** Meet the deliverables.
**Tasks:**
- Integration tests for the main flows (buy, auction, settlement, oracle).
- A full README: overview, setup, run (Docker Compose), API examples, assumptions.
- The ADR: database choice, concurrency and idempotency plan, trade-offs, and next
  steps.
- Make sure `docker compose up` runs the whole system end to end.
**Acceptance:** Fresh clone → `docker compose up` → all documented flows work;
tests pass.

---

## Checklist (verify before submitting)
- [x] Legendary uniqueness enforced in the database (no double sale possible).
- [x] Wallet never goes negative; `Available = Total − Reserved` always holds.
- [x] Daily spending limit enforced per guild.
- [x] +5% bid rule, no self-bid, cancel-only-if-not-top.
- [x] One active auction per legendary (partial unique index).
- [x] Anti-snipe 5-minute extension works.
- [x] All money moves written to the ledger and they reconcile.
- [x] Oracle failures (slow, zero, negative, wrong) handled well.
- [x] State-changing endpoints are idempotent.
- [x] Passes `go test -race ./...`.
- [x] README, Docker Compose, and ADR are present.

---

## Submission (from the brief)
- Email: `arash.sadeghian.haghighi@gmail.com`
- Subject: `چالش فنی Golang – نام و نام خانوادگی` (Golang challenge – Full Name)
- Phone/WhatsApp: `09190058697` — https://wa.me/+989190058697
- Suggested effort: 1–3 working days (ask if you need more).
