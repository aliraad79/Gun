package models_test

import (
	"testing"

	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

func pp(v int64) models.Px  { return models.Px(v * 1_0000_0000) }
func qq(v int64) models.Qty { return models.Qty(v * 1_0000_0000) }

// captureL2 returns a sink + slice that the test can inspect after the
// scenario runs.
func captureL2() (*[]models.BookDelta, models.L2Sink) {
	out := &[]models.BookDelta{}
	return out, func(d models.BookDelta) {
		*out = append(*out, d)
	}
}

// Adding the first order at a new price level emits a single delta with
// the order's volume.
func TestL2_NewLevel_EmitsDeltaForFullVolume(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	deltas, sink := captureL2()
	ob.SetL2Sink(sink)

	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(3)})

	if assert.Len(t, *deltas, 1) {
		d := (*deltas)[0]
		assert.Equal(t, models.BUY, d.Side)
		assert.Equal(t, pp(100), d.Price)
		assert.Equal(t, qq(3), d.Qty)
		assert.Equal(t, uint64(1), d.Seq, "first emitted event in a fresh book should have seq=1")
	}
}

// Adding a second order at an existing level updates the aggregate qty.
func TestL2_StackingAtSameLevel_AccumulatesQty(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	deltas, sink := captureL2()
	ob.SetL2Sink(sink)

	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(2)})
	ob.Add(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(5)})

	if assert.Len(t, *deltas, 2) {
		assert.Equal(t, qq(2), (*deltas)[0].Qty)
		assert.Equal(t, qq(7), (*deltas)[1].Qty, "second delta should reflect the cumulative 2+5")
		assert.True(t, (*deltas)[1].Seq > (*deltas)[0].Seq, "seqs must be monotonic")
	}
}

// Cancelling the only order at a level emits a final delta with Qty == 0.
func TestL2_LastOrderCancelled_EmitsZeroQty(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: pp(200), Volume: qq(1)})

	deltas, sink := captureL2()
	ob.SetL2Sink(sink) // start capturing after the add

	assert.NoError(t, ob.Cancel(1))

	if assert.Len(t, *deltas, 1) {
		assert.Equal(t, models.SELL, (*deltas)[0].Side)
		assert.Equal(t, pp(200), (*deltas)[0].Price)
		assert.Equal(t, models.ZeroQty, (*deltas)[0].Qty,
			"qty==0 means the level was fully removed")
	}
}

// Matching a full level produces deltas for the consumed level (Qty=0).
func TestL2_FullLevelMatch_EmitsZeroQty(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: pp(100), Volume: qq(1)})

	deltas, sink := captureL2()
	ob.SetL2Sink(sink)

	taker := models.Order{
		ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: pp(100), Volume: qq(1),
	}
	_, _ = ob.MatchTaker(taker, false)

	if assert.Len(t, *deltas, 1) {
		assert.Equal(t, models.SELL, (*deltas)[0].Side)
		assert.Equal(t, pp(100), (*deltas)[0].Price)
		assert.Equal(t, models.ZeroQty, (*deltas)[0].Qty)
	}
}

// Partial match emits a delta with the *remaining* aggregate qty.
func TestL2_PartialMatch_EmitsResidualQty(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: pp(100), Volume: qq(5)})

	deltas, sink := captureL2()
	ob.SetL2Sink(sink)

	taker := models.Order{
		ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: pp(100), Volume: qq(2),
	}
	_, _ = ob.MatchTaker(taker, false)

	if assert.Len(t, *deltas, 1) {
		assert.Equal(t, qq(3), (*deltas)[0].Qty,
			"5 resting minus 2 filled leaves a level of 3")
	}
}

// TotalQty() reflects the aggregate at each level without walking the list.
func TestL2_TotalQtyMatchesSumOfOrders(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	ob.Add(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(2)})
	ob.Add(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(3)})
	ob.Add(models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: pp(100), Volume: qq(7)})

	if assert.Len(t, ob.Buy, 1) {
		assert.Equal(t, qq(12), ob.Buy[0].TotalQty())
	}
}
