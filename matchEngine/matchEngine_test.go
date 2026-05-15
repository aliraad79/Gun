package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// p builds a Px from a whole-number int (e.g., p(10000) = 10000.00000000).
func p(v int64) models.Px { return models.Px(v * 1_0000_0000) }

// q builds a Qty from a whole-number int.
func q(v int64) models.Qty { return models.Qty(v * 1_0000_0000) }

// newBookWith returns a fresh orderbook prepopulated by Add()-ing each
// fixture order. Using the public API keeps the tests honest: any
// invariant the engine relies on (orderIndex, byPrice map, ladder sort)
// is established by the same code path real orders go through.
func newBookWith(symbol string, orders ...models.Order) *models.Orderbook {
	ob := models.NewOrderbook(symbol)
	for _, o := range orders {
		ob.Add(o)
	}
	return ob
}

func TestMatchAndAddNewOrder_BuyLimitOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 2, Type: models.LIMIT, Side: models.BUY,
		Volume: q(1), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(2), matches[0].BuyId)
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, p(10000), matches[0].Price)
	assert.Equal(t, q(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMultipleMatchAndAddNewOrder_BuyLimitOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 4, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Volume: q(3), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 3)
	for _, match := range matches {
		assert.Equal(t, p(10000), match.Price)
		assert.Equal(t, q(1), match.Volume)
	}
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMatchAndAddNewOrder_SellLimitOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 2, Type: models.LIMIT, Side: models.SELL,
		Volume: q(1), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].BuyId)
	assert.Equal(t, int64(2), matches[0].SellId)
	assert.Equal(t, p(10000), matches[0].Price)
	assert.Equal(t, q(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMultipleMatchAndAddNewOrder_SellLimitOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 4, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL,
		Volume: q(3), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 3)
	for _, match := range matches {
		assert.Equal(t, p(10000), match.Price)
		assert.Equal(t, q(1), match.Volume)
	}
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestCancelOrder_Success(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
	)

	err := matchEngine.CancelOrder(orderbook, 1)

	assert.NoError(t, err)
	assert.Empty(t, orderbook.Buy)
}

func TestCancelInvalidOrder_Success(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
	)

	err := matchEngine.CancelOrder(orderbook, 2)

	assert.ErrorIs(t, err, models.ErrCancelOrderFailed)
	if assert.Len(t, orderbook.Buy, 1) {
		assert.Equal(t, 1, orderbook.Buy[0].Orders.Len())
	}
}

func TestMatchAndAddNewOrder_sellMarketOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 2, Type: models.MARKET, Side: models.SELL,
		Volume: q(1), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].BuyId)
	assert.Equal(t, int64(2), matches[0].SellId)
	assert.Equal(t, p(10000), matches[0].Price)
	assert.Equal(t, q(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMatchAndAddNewOrder_buyMarketOrder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)},
	)

	newOrder := models.Order{
		ID: 2, Type: models.MARKET, Side: models.BUY,
		Volume: q(1), Price: p(10000),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, int64(2), matches[0].BuyId)
	assert.Equal(t, p(10000), matches[0].Price)
	assert.Equal(t, q(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}
