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
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: p(100),
				Orders: []models.Order{
					{ID: 1, Volume: q(1), Side: models.SELL, Price: p(100)},
					{ID: 2, Volume: q(1), Side: models.SELL, Price: p(100)},
					{ID: 3, Volume: q(1), Side: models.SELL, Price: p(100)},
				},
			},
		},
	}

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(1),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, taker)

	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].SellId, "oldest resting order (ID=1) must fill first")
	assert.Len(t, orderbook.Sell[0].Orders, 2)
	assert.Equal(t, int64(2), orderbook.Sell[0].Orders[0].ID)
	assert.Equal(t, int64(3), orderbook.Sell[0].Orders[1].ID)
}

// partial fill of a limit order: the leftover taker quantity must rest on the
// book at the unfilled remainder, NOT the original quantity.
func TestLimitMatch_PartialFillRestsRemainder(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: p(100),
				Orders: []models.Order{
					{ID: 1, Volume: q(2), Side: models.SELL, Price: p(100)},
				},
			},
		},
	}

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(5),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, taker)

	assert.Len(t, matches, 1)
	assert.Equal(t, q(2), matches[0].Volume)
	assert.Empty(t, orderbook.Sell, "fully consumed sell level should be removed")
	if assert.Len(t, orderbook.Buy, 1) {
		assert.Len(t, orderbook.Buy[0].Orders, 1)
		got := orderbook.Buy[0].Orders[0].Volume
		assert.Equal(t, q(3), got, "residual on book must be 3 (5 minus 2 filled)")
	}
}

// matching that fully consumes one price level and continues into the next
// used to crash pre-fix because the outer range was mutated mid-iteration.
func TestLimitMatch_SweepsMultiplePriceLevels(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: p(100),
				Orders: []models.Order{
					{ID: 1, Volume: q(1), Side: models.SELL, Price: p(100)},
				},
			},
			{
				Price: p(101),
				Orders: []models.Order{
					{ID: 2, Volume: q(1), Side: models.SELL, Price: p(101)},
				},
			},
			{
				Price: p(102),
				Orders: []models.Order{
					{ID: 3, Volume: q(1), Side: models.SELL, Price: p(102)},
				},
			},
		},
	}

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(101), Volume: q(10),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, taker)

	assert.Len(t, matches, 2, "should sweep the 100 and 101 levels and stop before 102")
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, int64(2), matches[1].SellId)
	if assert.Len(t, orderbook.Sell, 1) {
		assert.Equal(t, p(102), orderbook.Sell[0].Price,
			"only the 102 level should remain on the sell book")
	}
}

// regression for the negative-remainVolume arithmetic: when a single resting
// order is larger than the taker, the taker should fully fill and remain = 0,
// never overflow into a negative value that could fall through guards.
func TestLimitMatch_TakerFullyFilledByLargerResting(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: p(100),
				Orders: []models.Order{
					{ID: 1, Volume: q(10), Side: models.SELL, Price: p(100)},
				},
			},
		},
	}

	taker := models.Order{
		ID: 99, Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(3),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, taker)

	assert.Len(t, matches, 1)
	assert.Equal(t, q(3), matches[0].Volume)
	if assert.Len(t, orderbook.Sell, 1) && assert.Len(t, orderbook.Sell[0].Orders, 1) {
		got := orderbook.Sell[0].Orders[0].Volume
		assert.Equal(t, q(7), got, "resting volume must be 10-3=7")
		assert.False(t, got.IsNegative(), "resting volume must never go negative")
	}
	assert.Empty(t, orderbook.Buy, "fully filled taker must not rest on the book")
}

// market order across multiple levels: same structural mutation pattern as
// the limit-order sweep test, but going through handleMarketOrder.
func TestMarketMatch_SweepsMultiplePriceLevels(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: p(100),
				Orders: []models.Order{
					{ID: 1, Volume: q(1), Side: models.SELL, Price: p(100)},
				},
			},
			{
				Price: p(101),
				Orders: []models.Order{
					{ID: 2, Volume: q(1), Side: models.SELL, Price: p(101)},
				},
			},
		},
	}

	taker := models.Order{
		ID: 99, Type: models.MARKET, Side: models.BUY,
		Volume: q(2),
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, taker)

	assert.Len(t, matches, 2)
	assert.Empty(t, orderbook.Sell, "both levels fully consumed")
}
