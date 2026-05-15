package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// STPCancelTaker: same-user resting halts matching; taker remainder is
// dropped; the resting order stays on the book.
func TestSTP_CancelTaker_HaltsMatching(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(3)},
		// a third-party order at a worse price
		models.Order{ID: 2, Symbol: "BTC_USDT", UserID: 99, Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(5)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 42,
		Type: models.LIMIT, Side: models.BUY,
		STP:  models.STPCancelTaker,
		Price: p(101), Volume: q(10),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches, "no match should occur — own order halts the taker")
	if assert.Len(t, ob.Sell, 2) {
		// own order at 100 must still be there, untouched
		assert.Equal(t, q(3), ob.Sell[0].Orders.Head().Order.Volume)
		// third-party at 101 also untouched
		assert.Equal(t, q(5), ob.Sell[1].Orders.Head().Order.Volume)
	}
}

// STPCancelResting: same-user resting is cancelled, taker proceeds and
// matches against the next resting (here a third party at the same level).
func TestSTP_CancelResting_RemovesOwnAndContinues(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(3)},
		models.Order{ID: 2, Symbol: "BTC_USDT", UserID: 99, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(2)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 42,
		Type: models.LIMIT, Side: models.BUY,
		STP:  models.STPCancelResting,
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	if assert.Len(t, res.Matches, 1, "should match against the third-party resting only") {
		assert.Equal(t, int64(2), res.Matches[0].SellId, "match must be vs the third party")
		assert.Equal(t, q(2), res.Matches[0].Volume)
	}
	// own resting was cancelled, third-party fully consumed. Remaining
	// taker (5 - 2 = 3) rests at price 100.
	assert.Empty(t, ob.Sell, "sell side should be drained")
	if assert.Len(t, ob.Buy, 1) {
		assert.Equal(t, q(3), ob.Buy[0].Orders.Head().Order.Volume)
	}
}

// STPCancelBoth: cancels the resting AND drops the taker remainder.
func TestSTP_CancelBoth(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(3)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 42,
		Type: models.LIMIT, Side: models.BUY,
		STP:  models.STPCancelBoth,
		Price: p(100), Volume: q(5),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches)
	assert.Empty(t, ob.Sell, "resting must be cancelled")
	assert.Empty(t, ob.Buy, "taker remainder must be dropped")
}

// STPDecrement: both reduce by min(taker, resting); no trade reported.
// Then taker continues against the next book entry.
func TestSTP_Decrement_NetsOutThenContinues(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(2)},
		models.Order{ID: 2, Symbol: "BTC_USDT", UserID: 99, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(3)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 42,
		Type: models.LIMIT, Side: models.BUY,
		STP:  models.STPDecrement,
		Price: p(100), Volume: q(4),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	// own resting (2) is netted-out against taker -> no match, both reduced by 2.
	// taker now has 4-2 = 2 left. Crosses third party (3) -> match of 2.
	// third party has 3-2 = 1 left.
	assert.True(t, res.Accepted)
	if assert.Len(t, res.Matches, 1, "only the third-party crossing produces a real match") {
		assert.Equal(t, int64(2), res.Matches[0].SellId)
		assert.Equal(t, q(2), res.Matches[0].Volume)
	}
	if assert.Len(t, ob.Sell, 1) {
		assert.Equal(t, q(1), ob.Sell[0].Orders.Head().Order.Volume,
			"third party should have 1 remaining")
		assert.Equal(t, int64(2), ob.Sell[0].Orders.Head().Order.ID)
	}
}

// UserID == 0 (legacy / anonymous) disables STP regardless of mode.
func TestSTP_AnonymousOrdersMatchNormally(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 0, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 0,
		Type: models.LIMIT, Side: models.BUY,
		STP:  models.STPCancelTaker, // explicitly set but UserID is 0
		Price: p(100), Volume: q(1),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Len(t, res.Matches, 1, "UserID 0 disables STP — normal match should happen")
}

// Setting UserID without STP -> Normalize() picks STPCancelTaker (safe default).
func TestSTP_NormalizeDefaultsForUserOrders(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
	)

	taker := models.Order{
		ID: 100, Symbol: "BTC_USDT", UserID: 42,
		Type: models.LIMIT, Side: models.BUY,
		// STP intentionally left empty -> normalizes to cancel_taker
		Price: p(100), Volume: q(1),
	}

	res := matchEngine.MatchAndAddNewOrder(ob, taker)

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches, "default STP should prevent the same-user trade")
}
