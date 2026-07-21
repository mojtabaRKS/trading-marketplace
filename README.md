# Market Dragon

Market Dragon is a backend for a game marketplace, written in Go.
Guilds (player groups) buy and sell items in the land of Aethoria.

- **Common** and **Rare** items are sold at a fixed price (a "limit order").
- **Legendary** items are unique. They are sold only through auctions.

The system gives strong safety promises. It never sells one item twice.
It never lets a guild spend more money than it has. It handles repeated
requests safely. It keeps working even when the external price service is slow
or sends bad data.

> More docs:
> [`docs/IMPLEMENTATION_GUIDE.md`](docs/IMPLEMENTATION_GUIDE.md) (build plan) ·
> [`docs/ADR.md`](docs/ADR.md) (design choices and trade-offs) ·
> [`docs/DESIGN_FA.md`](docs/DESIGN_FA.md) (Persian write-up / مستند فارسی)

---

## Main features

- **No double sale.** The database makes sure one item is never sold twice.
- **No over-spending.** A wallet tracks `Available = Total − Reserved`. Every
  money change is locked and written to a ledger (a full history of money moves).
  Each guild also has a daily spending limit.
- **Auctions.** They last 24 hours. Each new bid must be at least 5% higher.
  A late bid extends the auction (anti-snipe). Losing bids are freed at once.
  A background job closes auctions when time is up.
- **Safe price feed.** The Oracle price service is called with a timeout, retries,
  and a circuit breaker (a switch that stops calls to a broken service). Prices
  are checked, and the last good price is kept if the feed fails.
- **Idempotency.** Idempotency means a repeated request has the same effect as
  one request. State-changing endpoints accept an `Idempotency-Key` header, so
  retries never apply twice.

## Tech stack

Go 1.26 · Gin (web framework) · GORM (database library) · PostgreSQL 16 ·
golang-migrate (database migrations) · Cobra + Viper (CLI and config) ·
slog (logging) · Docker Compose.

## Architecture

The code has three layers: `api → service → model`.
Shared clients live under `infra/`. Business rules live in `service/`, which also
owns all database queries, writes, and transactions. The `model/` layer is
behaviour-free: it holds only the models and seed data. The most important safety
rules are enforced by the database.

```
cmd/marketd/        CLI: serve | migrate | seed
migrations/         SQL files for the database schema
internal/
  config/           config from env vars and .env
  api/              web router, handlers, server start/stop
    middleware/     logging, recovery, idempotency
  service/          use-cases and rules; owns queries, writes, and transactions
  model/            database models + seed data (no data-access logic)
  infra/
    database/       database client + migration runner
    logging/        logger setup
    oracle/         price feed: interface, mock, resilient client, breaker
  worker/           auction settlement job + oracle poller
```

Note: a "migration" is a versioned change to the database schema.
A "transaction" is a group of database changes that all succeed or all fail.

---

## Quick start (Docker Compose)

This starts PostgreSQL and the API. It runs migrations and loads demo data.

```bash
docker compose up --build
# API on http://localhost:8080  (PostgreSQL is published on host port 5433)
curl -s localhost:8080/health
```

Stop it (add `-v` to also delete the database volume):

```bash
docker compose down
```

## Run locally (without the app container)

```bash
cp .env.example .env         # defaults point to Compose PostgreSQL on port 5433
docker compose up -d db      # start only the database
make run                     # go run ./cmd/marketd serve
# load demo data first, if you want:
DB_PORT=5433 go run ./cmd/marketd seed
```

### CLI commands

```bash
marketd serve      # run the API (runs migrations unless AUTO_MIGRATE=false; seeds if SEED=true)
marketd migrate    # apply database migrations
marketd seed       # load demo data
```

---

## Configuration

Config comes from environment variables, or from a `.env` file.
Order of priority: `env var > .env > default`.
See [`.env.example`](.env.example) for the full list.

| Variable | Default | Purpose |
|---|---|---|
| `HTTP_PORT` | `8080` | Port for the API |
| `DB_HOST` / `DB_PORT` | `localhost` / `5433` | PostgreSQL address (Compose uses 5433) |
| `DB_USER` / `DB_PASSWORD` / `DB_NAME` | `marketd` | PostgreSQL login |
| `AUTO_MIGRATE` | `true` | Run migrations on `serve` |
| `SEED` | `false` | Load demo data on `serve` |
| `AUCTION_WINDOW` | `24h` | Default auction length |
| `AUCTION_EXTENSION` | `5m` | Anti-snipe window and extension |
| `SETTLE_INTERVAL` | `10s` | How often the settlement job runs |
| `ORACLE_POLL_INTERVAL` | `30s` | How often prices are refreshed |
| `ORACLE_TIMEOUT` / `ORACLE_MAX_RETRIES` / `ORACLE_BACKOFF` | `2s` / `2` / `100ms` | Oracle call safety |
| `ORACLE_BREAKER_TRIP` / `ORACLE_BREAKER_COOLDOWN` | `3` / `15s` | Circuit breaker |
| `ORACLE_MAX_PRICE` / `ORACLE_MAX_DEVIATION` | `1e9` / `0` | Price checks (0 turns off the change check) |

---

## Demo data

`marketd seed` (or `SEED=true`) loads these guilds:

| Guild | Wallet | Daily limit |
|---|---|---|
| 1 Emberforge | 500,000 | 1,000,000 |
| 2 Stormhaven | 750,000 | 1,000,000 |
| 3 Nightspire | 2,000,000 | unlimited |

Items: `1` Iron Dagger (common, guild 1), `2` Elven Bow (rare, guild 2),
`3` Soul Reaver (legendary, guild 3), `4` Eye of the Dragon (legendary, guild 1).

---

## API

All money values are integers in "minor units" (the smallest coin unit, like cents).
State-changing requests accept an optional `Idempotency-Key` header.

### Interactive docs (Swagger)

The API is documented with OpenAPI. When the server runs, open the Swagger UI:

```
http://localhost:8080/swagger/index.html
```

The raw spec lives in [`docs/swagger.yaml`](docs/swagger.yaml) and
[`docs/swagger.json`](docs/swagger.json). To regenerate it from the code
annotations after you change a handler, run:

```bash
make swagger
```

### Endpoints

| Method | Path | What it does |
|---|---|---|
| `GET` | `/health` | Liveness probe |
| `POST` | `/items` | Register a new item (not for sale yet) |
| `GET` | `/items` | List items with live Oracle price |
| `GET` | `/items/{id}` | Item details + active offer |
| `POST` | `/items/{id}/list` | Offer a Common/Rare item at a fixed price |
| `POST` | `/items/{id}/auction` | Open an auction for a Legendary item |
| `POST` | `/items/{id}/buy` | Buy a fixed-price item |
| `POST` | `/items/{id}/bid` | Place a bid on the item's auction |
| `DELETE` | `/items/{id}/bid/{bid_id}` | Cancel a bid (not while highest bidder) |
| `GET` | `/auctions` | List active auctions |
| `GET` | `/auctions/{id}` | Auction details + highest bid |
| `GET` | `/guilds/{id}/wallet` | Wallet balance (total, reserved, available) |

### Items and limit orders (Common and Rare)

```bash
# Register a new item into the market (owned by a guild, not yet for sale).
curl -X POST localhost:8080/items \
  -H 'Content-Type: application/json' \
  -d '{"owner_guild_id":2,"name":"Elven Bow","tier":"rare","stock":5}'

# Offer it for sale at a fixed price (a limit order).
curl -X POST localhost:8080/items/2/list \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: list-elven-bow-1' \
  -d '{"seller_guild_id":2,"price":500}'

# Buy it. This checks the balance and the daily limit,
# moves the money, and transfers the item, all in one transaction.
curl -X POST localhost:8080/items/2/buy \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: buy-item-2' \
  -d '{"buyer_guild_id":1}'

# Read items (each carries its current checked Oracle price).
curl localhost:8080/items
curl localhost:8080/items/2
```

### Auctions (Legendary only)

```bash
# Open an auction. The seller must own the legendary item.
# Only one active auction per item is allowed.
curl -X POST localhost:8080/items/3/auction \
  -H 'Content-Type: application/json' \
  -d '{"seller_guild_id":3}'

# Place a bid. This reserves the money, applies the +5% rule,
# frees the previous top bidder, and extends the auction if it is late.
curl -X POST localhost:8080/items/3/bid \
  -H 'Content-Type: application/json' \
  -d '{"bidder_guild_id":1,"amount":260000}'

# Cancel a bid. Allowed only if you are not the top bidder.
curl -X DELETE localhost:8080/items/3/bid/1 \
  -H 'Content-Type: application/json' \
  -d '{"bidder_guild_id":1}'

# Read endpoints
curl localhost:8080/auctions
curl localhost:8080/auctions/1
```

When an auction ends, a background job closes it. The winner pays from the
reserved money, the seller gets paid, and the item moves to the winner.

### Wallet

```bash
curl localhost:8080/guilds/1/wallet   # total, reserved, and available balance
```

---

## Testing

```bash
make test              # unit tests (with the race detector)
make test-integration  # service-level integration tests against PostgreSQL on 5433
make test-e2e          # HTTP end-to-end tests: drive the full router over the DB
make test-report       # run everything and write a visual test-report.html
```

Integration and e2e tests need a running PostgreSQL (`docker compose up -d db`).

- **Integration tests** check the service layer: one sale under concurrent buys,
  the daily limit, no over-spending in the wallet, auction bid rules and money
  moves, anti-snipe, settlement (winner, no bids, and repeat runs), oracle checks
  with last good price, and repeated requests.
- **End-to-end tests** drive the real HTTP API (router + handlers + services)
  and assert the business rules through the wire: list/buy, no double-sale, no
  self-purchase, no self-bid, the +5% rule, fund reservation, cancel rules, one
  active auction per item, and idempotent requests.
- **`make test-report`** runs unit + integration + e2e with `go test -json` and
  renders a self-contained `test-report.html` dashboard (per-package, per-test
  pass/fail and durations). Open it in a browser.

For a full list of tests, which business rule each one checks, and step-by-step
detail for the e2e tests, see [`docs/TESTING.md`](docs/TESTING.md).

---

## Assumptions

- The guild id is sent in the request body. There is no login or auth layer.
- Money is an `int64` in minor units. Callers agree on the unit size.
- The Oracle service is a mock behind the `Source` interface.
- The daily limit applies to fixed-price buys, not to auctions
  (see the ADR "Known trade-off").

See [`docs/ADR.md`](docs/ADR.md) for design choices, trade-offs, and next steps.
