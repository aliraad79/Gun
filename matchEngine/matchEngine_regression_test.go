package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// price-time priority: at the same price level, the oldest resting order
// must match first. Pre-fix, the inner loop iterated newest-first (LIFO).
func TestLimitMatch_FIFOAtSamePriceLevel(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(1),
	}

	res := matchEngine.MatchAndAddNewOrder(orderbook, taker)
	matches := res.Matches

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].SellId, "oldest resting order (ID=1) must fill first")

	// remaining FIFO chain should be 2 -> 3
	if assert.Len(t, orderbook.Sell, 1) {
		level := orderbook.Sell[0]
		assert.Equal(t, 2, level.Orders.Len())
		assert.Equal(t, int64(2), level.Orders.Head().Order.ID)
		assert.Equal(t, int64(3), level.Orders.Head().Next.Order.ID)
	}
}

// partial fill of a limit order: the leftover taker quantity must rest on the
// book at the unfilled remainder, NOT the original quantity.
func TestLimitMatch_PartialFillRestsRemainder(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(2)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(orderbook, taker)
	matches := res.Matches

	assert.Len(t, matches, 1)
	assert.Equal(t, q(2), matches[0].Volume)
	assert.Empty(t, orderbook.Sell, "fully consumed sell level should be removed")
	if assert.Len(t, orderbook.Buy, 1) {
		level := orderbook.Buy[0]
		if assert.Equal(t, 1, level.Orders.Len()) {
			got := level.Orders.Head().Order.Volume
			assert.Equal(t, q(3), got, "residual on book must be 3 (5 minus 2 filled)")
		}
	}
}

// matching that fully consumes one price level and continues into the next.
func TestLimitMatch_SweepsMultiplePriceLevels(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(102), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(101), Volume: q(10),
	}

	res := matchEngine.MatchAndAddNewOrder(orderbook, taker)
	matches := res.Matches

	assert.Len(t, matches, 2, "should sweep the 100 and 101 levels and stop before 102")
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, int64(2), matches[1].SellId)
	if assert.Len(t, orderbook.Sell, 1) {
		assert.Equal(t, p(102), orderbook.Sell[0].Price,
			"only the 102 level should remain on the sell book")
	}
}

// regression for the negative-remainVolume arithmetic.
func TestLimitMatch_TakerFullyFilledByLargerResting(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(10)},
	)

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(3),
	}

	res := matchEngine.MatchAndAddNewOrder(orderbook, taker)
	matches := res.Matches

	assert.Len(t, matches, 1)
	assert.Equal(t, q(3), matches[0].Volume)
	if assert.Len(t, orderbook.Sell, 1) && assert.Equal(t, 1, orderbook.Sell[0].Orders.Len()) {
		got := orderbook.Sell[0].Orders.Head().Order.Volume
		assert.Equal(t, q(7), got, "resting volume must be 10-3=7")
		assert.False(t, got.IsNegative(), "resting volume must never go negative")
	}
	assert.Empty(t, orderbook.Buy, "fully filled taker must not rest on the book")
}

// market order across multiple levels.
func TestMarketMatch_SweepsMultiplePriceLevels(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Type: models.MARKET, Side: models.BUY,
		Volume: q(2),
	}

	res := matchEngine.MatchAndAddNewOrder(orderbook, taker)
	matches := res.Matches

	assert.Len(t, matches, 2)
	assert.Empty(t, orderbook.Sell, "both levels fully consumed")
}

// O(1) cancel: cancelling a resting order should not require walking the
// book. We can't directly assert "O(1)" in a unit test, but we can assert
// the post-state and that the orderID index is cleared.
func TestCancel_RemovesOrderAndClearsIndex(t *testing.T) {
	orderbook := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(101), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(102), Volume: q(1)},
	)

	require := assert.New(t)
	require.NoError(matchEngine.CancelOrder(orderbook, 2))
	require.Len(orderbook.Buy, 2)

	// 101 level should be gone; only 102 (best) and 100 remain.
	prices := []models.Px{}
	for _, l := range orderbook.Buy {
		prices = append(prices, l.Price)
	}
	require.Equal([]models.Px{p(102), p(100)}, prices)

	// Cancelling the same id again must fail (index cleared).
	require.ErrorIs(matchEngine.CancelOrder(orderbook, 2), models.ErrCancelOrderFailed)
}
