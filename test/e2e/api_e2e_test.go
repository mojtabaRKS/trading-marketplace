//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// createListedItem registers a Common/Rare item owned by `owner` and lists it at
// `price`, returning the item ID.
func (a *app) createListedItem(owner uint64, tier string, price int64) uint64 {
	a.t.Helper()
	code, item := a.req(http.MethodPost, "/items", map[string]any{
		"owner_guild_id": owner, "name": "E2E " + tier, "tier": tier, "stock": 1,
	}, nil)
	if code != http.StatusCreated {
		a.t.Fatalf("create item: want 201, got %d (%v)", code, item)
	}
	id := uintField(a.t, item, "id")
	code, listing := a.req(http.MethodPost, fmt.Sprintf("/items/%d/list", id), map[string]any{
		"seller_guild_id": owner, "price": price,
	}, nil)
	if code != http.StatusCreated {
		a.t.Fatalf("list item: want 201, got %d (%v)", code, listing)
	}
	return id
}

// createAuctionedItem registers a Legendary item owned by `owner` and opens an
// auction, returning the item ID.
func (a *app) createAuctionedItem(owner uint64) uint64 {
	a.t.Helper()
	code, item := a.req(http.MethodPost, "/items", map[string]any{
		"owner_guild_id": owner, "name": "E2E Legendary", "tier": "legendary",
	}, nil)
	if code != http.StatusCreated {
		a.t.Fatalf("create legendary: want 201, got %d (%v)", code, item)
	}
	id := uintField(a.t, item, "id")
	code, auction := a.req(http.MethodPost, fmt.Sprintf("/items/%d/auction", id), map[string]any{
		"seller_guild_id": owner,
	}, nil)
	if code != http.StatusCreated {
		a.t.Fatalf("open auction: want 201, got %d (%v)", code, auction)
	}
	return id
}

func TestE2EHealth(t *testing.T) {
	a := setupApp(t)
	code, body := a.req(http.MethodGet, "/health", nil, nil)
	if code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("health: want 200 ok, got %d (%v)", code, body)
	}
}

// TestE2ELimitOrderFlow proves the fixed-price flow end to end: list, buy (money
// moves + item sold), no double-sale, and no self-purchase.
func TestE2ELimitOrderFlow(t *testing.T) {
	a := setupApp(t)
	const seller, buyer = uint64(95001), uint64(95002)
	a.createGuild(seller, 0, 0)
	a.createGuild(buyer, 100_000, 0)

	itemID := a.createListedItem(seller, "rare", 5_000)

	// Detail view shows the open listing.
	code, detail := a.req(http.MethodGet, fmt.Sprintf("/items/%d", itemID), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("get item: want 200, got %d", code)
	}
	if detail["listing"] == nil {
		t.Fatalf("expected an open listing in item detail: %v", detail)
	}

	// Buy it: 200 and marked sold.
	code, sold := a.req(http.MethodPost, fmt.Sprintf("/items/%d/buy", itemID), map[string]any{
		"buyer_guild_id": buyer,
	}, nil)
	if code != http.StatusOK || sold["status"] != "sold" {
		t.Fatalf("buy: want 200 sold, got %d (%v)", code, sold)
	}

	// Buyer wallet dropped by the price.
	code, w := a.req(http.MethodGet, fmt.Sprintf("/guilds/%d/wallet", buyer), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("wallet: want 200, got %d", code)
	}
	if got := intField(t, w, "total"); got != 95_000 {
		t.Fatalf("buyer total: want 95000, got %d", got)
	}

	// Second buy fails: nothing is left open (no double-sale).
	code, _ = a.req(http.MethodPost, fmt.Sprintf("/items/%d/buy", itemID), map[string]any{
		"buyer_guild_id": buyer,
	}, nil)
	if code != http.StatusNotFound {
		t.Fatalf("double buy: want 404, got %d", code)
	}

	// A guild cannot buy its own listing.
	self := a.createListedItem(seller, "common", 100)
	code, _ = a.req(http.MethodPost, fmt.Sprintf("/items/%d/buy", self), map[string]any{
		"buyer_guild_id": seller,
	}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("self purchase: want 400, got %d", code)
	}
}

// TestE2EAuctionRules proves the auction business rules over HTTP: no self-bid,
// the +5% minimum, fund reservation, and cancel-restriction on the top bidder,
// plus the single-active-auction guarantee.
func TestE2EAuctionRules(t *testing.T) {
	a := setupApp(t)
	const seller, b1, b2 = uint64(95101), uint64(95102), uint64(95103)
	a.createGuild(seller, 0, 0)
	a.createGuild(b1, 1_000_000, 0)
	a.createGuild(b2, 1_000_000, 0)

	itemID := a.createAuctionedItem(seller)

	bidPath := fmt.Sprintf("/items/%d/bid", itemID)

	// Rule 1: a guild cannot bid on its own item.
	code, _ := a.req(http.MethodPost, bidPath, map[string]any{"bidder_guild_id": seller, "amount": 100_000}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("self-bid: want 400, got %d", code)
	}

	// First valid bid reserves funds.
	code, bid1 := a.req(http.MethodPost, bidPath, map[string]any{"bidder_guild_id": b1, "amount": 100_000}, nil)
	if code != http.StatusCreated {
		t.Fatalf("first bid: want 201, got %d (%v)", code, bid1)
	}
	bid1ID := uintField(t, bid1, "id")

	// Rule 4: reservation is reflected in the wallet (available drops, total not).
	code, w := a.req(http.MethodGet, fmt.Sprintf("/guilds/%d/wallet", b1), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("wallet: want 200, got %d", code)
	}
	if r := intField(t, w, "reserved"); r != 100_000 {
		t.Fatalf("reserved: want 100000, got %d", r)
	}
	if av := intField(t, w, "available"); av != 900_000 {
		t.Fatalf("available: want 900000, got %d", av)
	}

	// Rule 2: a bid below +5% (need >=105000) is rejected.
	code, _ = a.req(http.MethodPost, bidPath, map[string]any{"bidder_guild_id": b2, "amount": 104_000}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("low bid: want 400, got %d", code)
	}

	// A bid exactly at +5% is accepted; b1 is released.
	code, bid2 := a.req(http.MethodPost, bidPath, map[string]any{"bidder_guild_id": b2, "amount": 105_000}, nil)
	if code != http.StatusCreated {
		t.Fatalf("valid outbid: want 201, got %d (%v)", code, bid2)
	}
	code, w = a.req(http.MethodGet, fmt.Sprintf("/guilds/%d/wallet", b1), nil, nil)
	if r := intField(t, w, "reserved"); r != 0 {
		t.Fatalf("outbid release: want reserved 0, got %d", r)
	}

	// Rule 3: the current highest bidder cannot cancel.
	code, _ = a.req(http.MethodDelete, fmt.Sprintf("/items/%d/bid/%d", itemID, uintField(t, bid2, "id")),
		map[string]any{"bidder_guild_id": b2}, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("cancel highest: want 400, got %d", code)
	}

	// A non-highest bidder may cancel.
	code, _ = a.req(http.MethodDelete, fmt.Sprintf("/items/%d/bid/%d", itemID, bid1ID),
		map[string]any{"bidder_guild_id": b1}, nil)
	if code != http.StatusOK {
		t.Fatalf("cancel non-highest: want 200, got %d", code)
	}

	// Rule 5: only one active auction per item.
	code, _ = a.req(http.MethodPost, fmt.Sprintf("/items/%d/auction", itemID),
		map[string]any{"seller_guild_id": seller}, nil)
	if code != http.StatusConflict {
		t.Fatalf("second auction: want 409, got %d", code)
	}
}

// TestE2EIdempotentBuy proves a repeated request with the same Idempotency-Key
// has a single effect (money moves once) and replays the same response.
func TestE2EIdempotentBuy(t *testing.T) {
	a := setupApp(t)
	const seller, buyer = uint64(95201), uint64(95202)
	a.createGuild(seller, 0, 0)
	a.createGuild(buyer, 100_000, 0)

	itemID := a.createListedItem(seller, "rare", 7_000)
	// Unique per run: idempotency keys persist in the DB across test runs.
	headers := map[string]string{"Idempotency-Key": fmt.Sprintf("e2e-buy-%d", time.Now().UnixNano())}

	code1, body1 := a.req(http.MethodPost, fmt.Sprintf("/items/%d/buy", itemID), map[string]any{"buyer_guild_id": buyer}, headers)
	code2, body2 := a.req(http.MethodPost, fmt.Sprintf("/items/%d/buy", itemID), map[string]any{"buyer_guild_id": buyer}, headers)

	if code1 != http.StatusOK || code2 != http.StatusOK {
		t.Fatalf("idempotent buy: want 200/200, got %d/%d", code1, code2)
	}
	if fmt.Sprint(body1) != fmt.Sprint(body2) {
		t.Fatalf("replayed body differs: %v vs %v", body1, body2)
	}

	// Money moved exactly once.
	code, w := a.req(http.MethodGet, fmt.Sprintf("/guilds/%d/wallet", buyer), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("wallet: want 200, got %d", code)
	}
	if got := intField(t, w, "total"); got != 93_000 {
		t.Fatalf("buyer total after idempotent buys: want 93000, got %d", got)
	}
}

// TestE2EItemPriceFromOracle proves the live Oracle price appears in the item
// read endpoints.
func TestE2EItemPriceFromOracle(t *testing.T) {
	a := setupApp(t)
	const owner = uint64(95301)
	a.createGuild(owner, 0, 0)

	code, item := a.req(http.MethodPost, "/items", map[string]any{
		"owner_guild_id": owner, "name": "Priced", "tier": "rare", "stock": 1,
	}, nil)
	if code != http.StatusCreated {
		t.Fatalf("create item: want 201, got %d", code)
	}
	itemID := uintField(t, item, "id")

	a.setPrice(itemID, 4_242)

	code, detail := a.req(http.MethodGet, fmt.Sprintf("/items/%d", itemID), nil, nil)
	if code != http.StatusOK {
		t.Fatalf("get item: want 200, got %d", code)
	}
	itemObj, ok := detail["item"].(map[string]any)
	if !ok {
		t.Fatalf("item object missing: %v", detail)
	}
	if got := intField(t, itemObj, "price"); got != 4_242 {
		t.Fatalf("oracle price: want 4242, got %d", got)
	}
}
