package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// IOC: match what crosses, drop the rest; nothing rests on the book.
func TestTIF_IOC_DropsUnfilledRemainder(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.IOC,
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted, "IOC with partial fill is still accepted")
	assert.Len(t, res.Matches, 1, "should have filled against the resting sell")
	assert.Equal(t, q(1), res.Matches[0].Volume)
	assert.Empty(t, ob.Buy, "IOC must not rest the remainder on the book")
}

// IOC against an empty side: zero matches, accepted, no rest.
func TestTIF_IOC_AcceptedWithNoMatches(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")

	taker := models.Order{
		ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.IOC,
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches)
	assert.Empty(t, ob.Buy)
}

// FOK accepted: enough crossing volume exists, order matches in full.
func TestTIF_FOK_FullyFillable(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(3)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(2)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.FOK,
		Price: p(101), Volume: q(5), // exactly 3 + 2 available at <= 101
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Len(t, res.Matches, 2)
	assert.Empty(t, ob.Sell, "both resting sells fully consumed")
	assert.Empty(t, ob.Buy, "FOK never rests")
}

// FOK rejected: not enough crossing volume; no state change at all.
func TestTIF_FOK_NotFullyFillable_Rejects(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(2)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.FOK,
		Price: p(100), Volume: q(5), // only 2 available
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.False(t, res.Accepted, "FOK that can't fully fill must be rejected")
	assert.Equal(t, matchEngine.RejectFOKUnfillable, res.Reason)
	assert.Empty(t, res.Matches, "rejected FOK must produce zero partial fills")
	if assert.Len(t, ob.Sell, 1) {
		assert.Equal(t, 1, ob.Sell[0].Orders.Len(),
			"the resting sell must be untouched after a FOK rejection")
		assert.Equal(t, q(2), ob.Sell[0].Orders.Head().Order.Volume)
	}
}

// Post-only accepted: order does NOT cross the spread; it rests as a maker.
func TestTIF_PostOnly_NonCrossingRests(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.PostOnly,
		Price: p(100), // below the best ask, so doesn't cross
		Volume: q(1),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches, "post-only that doesn't cross produces no matches")
	if assert.Len(t, ob.Buy, 1) {
		assert.Equal(t, p(100), ob.Buy[0].Price)
		assert.Equal(t, 1, ob.Buy[0].Orders.Len())
	}
}

// Post-only rejected: order would cross; reject before any matching.
func TestTIF_PostOnly_CrossingRejects(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.PostOnly,
		Price: p(100), Volume: q(1), // crosses (would take liquidity at 100)
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.False(t, res.Accepted)
	assert.Equal(t, matchEngine.RejectPostOnlyCrossed, res.Reason)
	assert.Empty(t, res.Matches)
	assert.Empty(t, ob.Buy, "rejected post-only must not rest")
	if assert.Len(t, ob.Sell, 1) {
		assert.Equal(t, 1, ob.Sell[0].Orders.Len(),
			"the resting sell must be untouched after a post-only rejection")
	}
}

// GTC (default) restitutes the remainder, as the existing tests assume.
// Smoke check that the new TIF plumbing doesn't change the default path.
func TestTIF_GTC_PartialFillRestsRemainder(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 99, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		// TimeInForce intentionally left empty -> normalizes to GTC
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Len(t, res.Matches, 1)
	if assert.Len(t, ob.Buy, 1) {
		assert.Equal(t, q(4), ob.Buy[0].Orders.Head().Order.Volume,
			"GTC must rest the unfilled 4-of-5 remainder")
	}
}
