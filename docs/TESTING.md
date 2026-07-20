# Testing Guide

This document explains the tests in this project. It says what each test does
and which business rule it checks. It gives extra detail for the end-to-end
(e2e) tests.

The tests are written in plain, simple English so a new team member can follow
them.

## Test types

There are three kinds of tests:

1. **Unit tests** — test small pure functions (the business rules) in memory.
   They are fast and need no database.
2. **Integration tests** — test the service layer against a real PostgreSQL.
   They check money, locks, and database rules.
3. **End-to-end (e2e) tests** — call the real HTTP API (router, handlers,
   services, and the database) the way a client would. They check the rules
   "through the wire".

## How to run

You need PostgreSQL running for integration and e2e tests:

```bash
docker compose up -d db
```

Then:

```bash
make test              # unit tests only (fast, no database)
make test-integration  # service-layer integration tests
make test-e2e          # HTTP end-to-end tests
make test-report       # run all tests and write test-report.html
```

`make test-report` builds a visual HTML report. Open `test-report.html` in a
browser to see each package and each test with a pass or fail badge and its
run time. Failing tests also show their output.

A "build tag" is a label that turns a file on or off at build time. Integration
files use `//go:build integration` and e2e files use `//go:build e2e`, so plain
`make test` skips them.

## Business rules and where we test them

The rules come from the challenge brief. Every rule has at least one test.

| Business rule | Tests | Level |
|---|---|---|
| 1. A guild cannot bid on its own item | `TestEnsureNotSelfBid`, `TestAuctionBidFlow`, `TestE2EAuctionRules` | unit / integration / e2e |
| 2. A new bid must be at least 5% above the current highest bid | `TestMinNextBid`, `TestEnsureBidBeatsCurrent`, `TestAuctionBidFlow`, `TestE2EAuctionRules` | unit / integration / e2e |
| 3. You can cancel a bid only if you are not the highest bidder | `TestEnsureCanCancelBid`, `TestAuctionBidFlow`, `TestE2EAuctionRules` | unit / integration / e2e |
| 4. Check funds (including reserves) before any money move | `TestEnsureCanReserve`, `TestWalletConcurrentReserveNoOvercommit`, `TestWalletReleaseExceedsReserved`, `TestE2EAuctionRules` | unit / integration / e2e |
| 5. Each Legendary has at most one active auction | `TestListLegendaryRejected`, `TestE2EAuctionRules` | integration / e2e |
| No invalid or duplicate sale of an asset | `TestBuyConcurrentSellsOnce`, `TestE2ELimitOrderFlow` | integration / e2e |
| A guild cannot buy its own listing | `TestE2ELimitOrderFlow` | e2e |
| Daily purchase cap | `TestEnsureWithinDailyCap`, `TestBuyDailyCapEnforced` | unit / integration |
| Placing a bid reserves funds; a losing bid is released | `TestAuctionBidFlow`, `TestBidConcurrentConsistency`, `TestE2EAuctionRules` | integration / e2e |
| Anti-snipe: a late bid extends the auction | `TestMaybeExtend`, `TestAuctionAntiSnipeExtends` | unit / integration |
| Auction end with a winner / with no bids | `TestSettleAuctionWithWinner`, `TestSettleAuctionNoBids`, `TestSettleDueSkipsOpenAuctions` | integration |
| Wallet is auditable and always balances | `TestWalletMovementsReconcile` | integration |
| Oracle may be wrong/slow; keep last known good | `TestValidatePrice`, `TestOracleRefreshKeepsLastKnownGood`, `TestOracleLoadLastKnownGood`, `TestCircuitBreaker*`, `TestResilientClient*` | unit / integration |
| Survive duplicate operations (idempotency) | `TestIdempotency*`, `TestE2EIdempotentBuy` | integration / e2e |

## Unit tests (no database)

Files: `internal/service/rules_test.go`, `internal/service/oracle_test.go`,
`internal/infra/oracle/client_test.go`.

- `TestAvailableBalance` — available balance is total minus reserved.
- `TestEnsureCanReserve` — you can reserve only up to the available balance.
- `TestEnsureNotSelfBid` — a seller cannot bid on their own item.
- `TestMinNextBid` — the smallest next bid is the current bid plus 5%.
- `TestEnsureBidBeatsCurrent` — a new bid must reach the +5% floor.
- `TestEnsureCanCancelBid` — the highest bidder cannot cancel.
- `TestEnsureWithinDailyCap` — a purchase must stay under the daily cap.
- `TestMaybeExtend` — a bid in the last minutes extends the auction.
- `TestAuctionEnded` — the auction is closed after its end time.
- `TestValidatePrice`, `TestValidatePriceDisabledGuards` — the Oracle price
  must be positive and inside plausible limits.
- `TestCircuitBreakerTripsAndRecovers`, `TestResilientClientRetriesThenSucceeds`,
  `TestResilientClientOpensCircuitAfterFailures`,
  `TestResilientClientTimesOutSlowSource` — the Oracle client retries, times
  out, and "opens the circuit" when the feed keeps failing. A circuit breaker
  is a switch that stops calls to a failing service for a short time.

## Integration tests (real PostgreSQL)

Files: `internal/service/*_integration_test.go`,
`internal/api/middleware/idempotency_integration_test.go`.

- `TestWalletConcurrentReserveNoOvercommit` — many reserves run at the same
  time; the wallet never goes below zero available.
- `TestWalletMovementsReconcile` — after many moves, the wallet total and the
  ledger still agree.
- `TestWalletReleaseExceedsReserved` — you cannot release more than you reserved.
- `TestBuyConcurrentSellsOnce` — many buyers try the same item; only one wins.
- `TestBuyDailyCapEnforced` — a buy over the daily cap is rejected.
- `TestListLegendaryRejected` — a Legendary cannot be a fixed-price listing.
- `TestAuctionBidFlow` — a full bid path: reserve funds, reject self-bid and
  low bids, release the previous top bidder, and block cancel by the leader.
- `TestBidConcurrentConsistency` — many bids at once leave one clear winner and
  correct reserves.
- `TestAuctionAntiSnipeExtends` — a bid near the end moves the end time later.
- `TestSettleAuctionWithWinner` — at close, the winner pays from reserved funds,
  the seller is paid, and the item moves to the winner.
- `TestSettleAuctionNoBids` — with no bids, the item becomes available again.
- `TestSettleDueSkipsOpenAuctions` — only auctions past their end time settle.
- `TestOracleRefreshKeepsLastKnownGood` — a bad price is rejected and the last
  good price is kept.
- `TestOracleLoadLastKnownGood` — on start, the service loads the last good
  price from the database.
- `TestIdempotencyReplaysStoredResponse` — the same idempotency key returns the
  stored answer and does not run the work twice.
- `TestIdempotencyRejectsDifferentBody` — the same key with a different body is
  rejected.
- `TestIdempotencyConcurrentSingleEffect` — the same key sent many times at once
  runs the work only once.

## End-to-end (e2e) tests — details

Files: `test/e2e/helpers_test.go`, `test/e2e/api_e2e_test.go`.

These tests build the real router with real services and a real database. Each
request is a real HTTP call. The tests use dedicated high guild IDs (like
`95001`) so their data does not clash with the seed data, and they reset those
guilds and wallets before each run.

### Setup helpers

- `setupApp` — wires the wallet, item, listing, auction, and oracle services,
  builds the router, and returns a small helper object.
- `req` — sends an HTTP request and returns the status code and the JSON body.
- `createGuild` — resets and creates a guild with a funded wallet.
- `setPrice` — makes the mock Oracle report a price for an item.
- `createListedItem` / `createAuctionedItem` — quick ways to put an item up for
  sale or open an auction.

### `TestE2EHealth`

Calls `GET /health` and checks the status is `200` with body `{"status":"ok"}`.
This proves the server and router are wired correctly.

### `TestE2ELimitOrderFlow`

Checks the fixed-price flow for Common/Rare items.

Steps:
1. Create a seller (no money) and a buyer (100,000).
2. Register a Rare item for the seller and list it for 5,000.
3. `GET /items/{id}` and check the open listing appears in the detail.
4. `POST /items/{id}/buy` as the buyer — expect `200` and status `sold`.
5. `GET /guilds/{buyer}/wallet` — the total dropped by 5,000 (now 95,000).
6. Buy the same item again — expect `404`, because nothing is open anymore.
7. List a new item and try to buy it as the seller — expect `400`.

Rules checked: no duplicate sale (step 6), money moves correctly (step 5), and
a guild cannot buy its own listing (step 7).

### `TestE2EAuctionRules`

Checks the auction rules for Legendary items.

Steps:
1. Create a seller (no money) and two bidders (1,000,000 each).
2. Register a Legendary item and open an auction.
3. Seller tries to bid — expect `400` (rule 1: no self-bid).
4. Bidder 1 bids 100,000 — expect `201`.
5. `GET /guilds/{bidder1}/wallet` — reserved is 100,000 and available is
   900,000 (rule 4: funds are reserved, not spent).
6. Bidder 2 bids 104,000 — expect `400`, because it is below the +5% floor of
   105,000 (rule 2).
7. Bidder 2 bids 105,000 — expect `201`. Bidder 1 is now released, so their
   reserved amount is 0.
8. Bidder 2 (the leader) tries to cancel their bid — expect `400` (rule 3).
9. Bidder 1 (not the leader) cancels their old bid — expect `200`.
10. Open a second auction for the same item — expect `409` (rule 5: one active
    auction per item).

Rules checked: 1, 2, 3, 4, 5, plus fund reservation and release.

### `TestE2EIdempotentBuy`

Checks that a repeated request has a single effect.

Steps:
1. Create a seller and a buyer (100,000). List an item for 7,000.
2. Send `POST /items/{id}/buy` twice with the same `Idempotency-Key`.
3. Both calls return `200` and the same body.
4. `GET /guilds/{buyer}/wallet` — the total dropped by 7,000 only once
   (now 93,000).

The key is made unique per run, because idempotency keys are stored in the
database and stay there between runs.

Rule checked: the market survives duplicate operations.

### `TestE2EItemPriceFromOracle`

Checks that the live Oracle price shows in the item read endpoints.

Steps:
1. Register an item.
2. Make the mock Oracle report a price of 4,242 for that item and refresh.
3. `GET /items/{id}` — the item's `price` field is 4,242.

Rule checked: items are listed with the current checked price.
