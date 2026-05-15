package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// Modify with same price + smaller quantity keeps the FIFO queue position.
func TestModify_SamePriceSmallerQty_PreservesQueuePosition(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(5)}, // will be modified
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
	)

	res := matchEngine.ModifyOrder(ob, 2, p(100), q(3))

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches, "in-place reduction produces no matches")
	if assert.Len(t, ob.Buy, 1) && assert.Equal(t, 3, ob.Buy[0].Orders.Len()) {
		// FIFO must still be 1 -> 2 -> 3
		n := ob.Buy[0].Orders.Head()
		assert.Equal(t, int64(1), n.Order.ID)
		assert.Equal(t, int64(2), n.Next.Order.ID)
		assert.Equal(t, q(3), n.Next.Order.Volume, "ID=2's volume should now be 3")
		assert.Equal(t, int64(3), n.Next.Next.Order.ID)
	}
}

// Modify with a new price cancels and re-adds — queue position is lost
// AND the new order may match against the book.
func TestModify_PriceChange_CrossingMatches(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(102), Volume: q(2)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(99), Volume: q(2)},
	)

	// Move our buy from 99 up to 102 — it now crosses the resting sell.
	res := matchEngine.ModifyOrder(ob, 2, p(102), q(2))

	assert.True(t, res.Accepted)
	if assert.Len(t, res.Matches, 1) {
		assert.Equal(t, int64(1), res.Matches[0].SellId)
		assert.Equal(t, q(2), res.Matches[0].Volume)
	}
	assert.Empty(t, ob.Sell, "sell side fully consumed")
	assert.Empty(t, ob.Buy, "buy side fully consumed")
}

// Modify with a quantity increase loses FIFO position (cancel + re-add).
func TestModify_QtyIncrease_LosesQueuePosition(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)}, // will be increased
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
	)

	res := matchEngine.ModifyOrder(ob, 2, p(100), q(5))

	assert.True(t, res.Accepted)
	assert.Empty(t, res.Matches)
	if assert.Len(t, ob.Buy, 1) && assert.Equal(t, 3, ob.Buy[0].Orders.Len()) {
		// new FIFO: 1 -> 3 -> 2 (2 was re-added at the tail)
		n := ob.Buy[0].Orders.Head()
		assert.Equal(t, int64(1), n.Order.ID)
		assert.Equal(t, int64(3), n.Next.Order.ID)
		assert.Equal(t, int64(2), n.Next.Next.Order.ID)
		assert.Equal(t, q(5), n.Next.Next.Order.Volume)
	}
}

// Modify with newVolume = 0 is equivalent to cancel.
func TestModify_ZeroVolume_Cancels(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)},
	)

	res := matchEngine.ModifyOrder(ob, 1, p(100), models.ZeroQty)

	assert.True(t, res.Accepted)
	assert.Empty(t, ob.Buy)
}

// Modify on an unknown order ID is rejected.
func TestModify_UnknownOrderID_Rejected(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")

	res := matchEngine.ModifyOrder(ob, 999, p(100), q(1))

	assert.False(t, res.Accepted)
	assert.Equal(t, matchEngine.RejectModifyNotFound, res.Reason)
}
