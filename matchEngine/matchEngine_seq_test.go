package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

// Every produced match must carry a strictly monotonic sequence number
// drawn from the orderbook's counter.
func TestSeq_StrictlyMonotonicAcrossOrders(t *testing.T) {
	ob := newBookWith("BTC_USDT",
		models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)},
		models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(1)},
		models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(102), Volume: q(1)},
	)

	taker1 := models.Order{
		ID: 100, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: p(102), Volume: q(3),
	}
	res1 := matchEngine.MatchAndAddNewOrder(ob, taker1)

	if assert.Len(t, res1.Matches, 3) {
		assert.Equal(t, uint64(1), res1.Matches[0].Seq)
		assert.Equal(t, uint64(2), res1.Matches[1].Seq)
		assert.Equal(t, uint64(3), res1.Matches[2].Seq)
	}

	// New batch of liquidity, then a new taker.
	ob.Add(models.Order{ID: 10, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)})

	taker2 := models.Order{
		ID: 101, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price: p(100), Volume: q(1),
	}
	res2 := matchEngine.MatchAndAddNewOrder(ob, taker2)

	if assert.Len(t, res2.Matches, 1) {
		assert.Equal(t, uint64(4), res2.Matches[0].Seq, "seq must continue across separate match calls")
	}
}

// Seq survives the proto round-trip so recovery resumes at the next
// un-used value.
func TestSeq_PersistsAcrossProtoRoundTrip(t *testing.T) {
	ob := models.NewOrderbook("BTC_USDT")
	// drive seq to 5
	for i := 0; i < 5; i++ {
		ob.NextSeq()
	}
	assert.Equal(t, uint64(5), ob.Seq())

	clone := models.OrderbookFromProto(ob.ToProto())
	assert.Equal(t, uint64(5), clone.Seq())
	assert.Equal(t, uint64(6), clone.NextSeq(), "next seq after recovery must be 6")
}
