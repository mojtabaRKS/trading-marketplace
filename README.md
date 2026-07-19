# Market Dragon

A secure trading & auction marketplace backend for the land of **Aethoria**, written in Go.
Guilds trade **Common/Rare** items via fixed-price limit orders and compete for unique
**Legendary** items via auctions — with hard guarantees against double-sales, over-spending,
duplicate requests, and an unreliable external price feed.

> Full brief: [`docs/IMPLEMENTATION_GUIDE.md`](docs/IMPLEMENTATION_GUIDE.md) ·
> Design rationale & trade-offs: [`docs/ADR.md`](docs/ADR.md)

---

## Highlights

- **No invalid/duplicate sale of an asset** — enforced at the database (partial unique index for
  one active auction per legendary; row locks + status transitions for single-sale).
- **No over-spending** — wallet with `Available = Total − Reserved`, row-locked money movements,
  an append-only ledger (`wallet_transactions`), and per-guild daily purchase caps.
- **Auctions** — 24h window, +5% minimum increment, anti-snipe extension, immediate release of
  outbid reservations, and deterministic settlement by a background worker.
- **Resilient Oracle price feed** — timeout + retry/backoff + circuit breaker, price validation,
  and last-known-good fallback so a slow/wrong/zero/negative feed never corrupts state.
- **Idempotency** — `Idempotency-Key` on all state-changing endpoints; duplicates never double-apply.

## Tech stack

Go 1.26 · Gin · GORM (PostgreSQL 16) · golang-migrate · Cobra + Viper · slog · Docker Compose.

## Architecture

Layered: `api → service → repository`, with cross-cutting clients under `infra/`.
Business rules live in `service/`; the hardest invariants are enforced in the **database**.

```
cmd/marketd/        cobra CLI: serve | migrate | seed
migrations/         versioned SQL (golang-migrate), embedded via //go:embed
internal/
  config/           viper config (env > .env > default)
  api/              Gin router, handlers, server lifecycle
    middleware/     logging, recovery, idempotency
  service/          use-cases + pure rules; owns DB transaction boundaries
  repository/       GORM models, repositories, seed data
  infra/
    database/       GORM client + migration runner
    logging/        slog builder
    oracle/         price-feed Source, mock, resilient client, circuit breaker
  worker/           auction settlement ticker + oracle poller
```

---

## Quick start (Docker Compose)

Runs Postgres + the API, applies migrations, and seeds demo data automatically.

```bash
docker compose up --build
# API on http://localhost:8080  (Postgres published on host port 5433)
curl -s localhost:8080/healthz
```

Tear down (add `-v` to also drop the database volume):

```bash
docker compose down
```

## Run locally (without the app container)

```bash
cp .env.example .env         # defaults target Compose Postgres on port 5433
docker compose up -d db      # just the database
make run                     # go run ./cmd/marketd serve
# or seed demo data first:
DB_PORT=5433 go run ./cmd/marketd seed
```

### CLI

```bash
marketd serve      # run the HTTP API (auto-migrates unless AUTO_MIGRATE=false; seeds if SEED=true)
marketd migrate    # apply migrations
marketd seed       # load deterministic demo data
```

---

## Configuration

All config is read from environment variables (optionally a `.env` file). Precedence:
`env var > .env > built-in default`. See [`.env.example`](.env.example) for the full list.

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_PORT` | `8080` | HTTP listen port |
| `DB_HOST` / `DB_PORT` | `localhost` / `5433` | Postgres address (Compose publishes 5433) |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `marketd` | Postgres credentials |
| `AUTO_MIGRATE` | `true` | Apply migrations on `serve` |
| `SEED` | `false` | Load demo data on `serve` |
| `AUCTION_WINDOW` | `24h` | Default auction duration |
| `AUCTION_EXTENSION` | `5m` | Anti-snipe window & extension |
| `SETTLE_INTERVAL` | `10s` | Auction settlement tick |
| `ORACLE_POLL_INTERVAL` | `30s` | Price refresh interval |
| `ORACLE_TIMEOUT` / `ORACLE_MAX_RETRIES` / `ORACLE_BACKOFF` | `2s` / `2` / `100ms` | Oracle call resilience |
| `ORACLE_BREAKER_TRIP` / `ORACLE_BREAKER_COOLDOWN` | `3` / `15s` | Circuit breaker |
| `ORACLE_MAX_PRICE` / `ORACLE_MAX_DEVIATION` | `1e9` / `0` | Price validation (0 disables deviation guard) |

---

## Seed data

`marketd seed` (or `SEED=true`) loads:

| Guild | Wallet | Daily cap |
|---|---|---|
| 1 Emberforge | 500,000 | 1,000,000 |
| 2 Stormhaven | 750,000 | 1,000,000 |
| 3 Nightspire | 2,000,000 | unlimited |

Items: `1` Iron Dagger (common, guild 1), `2` Elven Bow (rare, guild 2),
`3` Soul Reaver (legendary, guild 3), `4` Eye of the Dragon (legendary, guild 1).

---

## API

All amounts are integer **minor units**. State-changing requests accept an optional
`Idempotency-Key` header.

### Health

```bash
curl localhost:8080/healthz
```

### Limit orders (Common & Rare)

```bash
# List a rare item for sale
curl -X POST localhost:8080/listings \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: list-elven-bow-1' \
  -d '{"seller_guild_id":2,"item_id":2,"price":500}'

# Buy it (checks available balance + daily cap, transfers ownership, one TX)
curl -X POST localhost:8080/listings/1/buy \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: buy-listing-1' \
  -d '{"buyer_guild_id":1}'
```

### Auctions (Legendary only)

```bash
# Start an auction (seller must own a legendary item; one active auction per item)
curl -X POST localhost:8080/auctions \
  -H 'Content-Type: application/json' \
  -d '{"seller_guild_id":3,"item_id":3}'

# Place a bid (reserves funds, +5% rule, releases the previous leader, anti-snipe)
curl -X POST localhost:8080/auctions/1/bids \
  -H 'Content-Type: application/json' \
  -d '{"bidder_guild_id":1,"amount":260000}'

# Cancel a non-winning bid
curl -X DELETE localhost:8080/auctions/1/bids/1 \
  -H 'Content-Type: application/json' \
  -d '{"bidder_guild_id":1}'

# Reads
curl localhost:8080/auctions/1
curl localhost:8080/auctions/1/bids
```

Expired auctions are settled automatically by the background worker: the winner pays from
reserved funds, the seller is credited, and the item transfers.

### Prices (Oracle)

```bash
curl localhost:8080/prices/3   # current validated base price for an item
```

---

## Testing

```bash
make test              # unit tests (-race)
make test-integration  # integration tests against Postgres on port 5433
```

Integration tests are tagged `//go:build integration` and require a running Postgres
(`docker compose up -d db`). They cover the critical flows: concurrent single-sale, daily-cap
enforcement, wallet no-over-commit, auction bidding rules + reserve movement + anti-snipe,
settlement (winner/no-bids/idempotent), oracle validation + last-known-good, and idempotent
duplicate requests.

---

## Assumptions

- Guild identity is supplied in the request body; there is no authn/authz layer.
- Money is `int64` minor units; callers agree on the unit scale.
- The Oracle upstream is mocked (`internal/infra/oracle`) behind the `Source` interface.
- The daily purchase cap applies to limit-order buys, not auction settlement (see ADR — Known trade-off).

See [`docs/ADR.md`](docs/ADR.md) for decisions, trade-offs, and "what we'd add with more time".
