package matchEngine_test

import (
	"math/rand/v2"
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
)

// BenchmarkPostOnly_NonCrossing measures the post-only pre-check on the
// happy path: the order doesn't cross, so the check returns false and
// the order rests. This is the cost the maker pays for the safety.
func BenchmarkPostOnly_NonCrossing(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(10_000, 1000, mid)

	taker := models.Order{
		Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		TimeInForce: models.PostOnly,
		Price:       p(mid - 2000), // well below best ask
		Volume:      q(1),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taker.ID = int64(i + 1)
		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkFOK_FullyFillable measures the FOK pre-flight scan when the
// crossing side has enough volume. The scan walks the opposite ladder
// summing volumes; cost scales with how many resting orders it has to
// touch to reach the taker's quantity.
func BenchmarkFOK_FullyFillable(b *testing.B) {
	mid := int64(10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ob := buildBook(200, 20, mid)
		taker := models.Order{
			ID: int64(i + 1_000_000), Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
			TimeInForce: models.FOK,
			Price:       p(mid + 10),
			Volume:      q(5),
		}
		b.StartTimer()

		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkSTP_DisabledByUserIDZero confirms the "no STP" fast path is
// cheap when UserID == 0. This is the dominant case for anonymous
// venues.
func BenchmarkSTP_DisabledByUserIDZero(b *testing.B) {
	mid := int64(10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ob := buildBook(100, 10, mid)
		taker := models.Order{
			ID: int64(i + 1_000_000), Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
			Price: p(mid + 1), Volume: models.Qty(1000_0000),
			// UserID intentionally zero -> STP disabled, fast path
		}
		b.StartTimer()

		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkSTP_SameUserCancelTaker exercises the STP check + the
// cancel-taker path when the taker meets its own resting order.
func BenchmarkSTP_SameUserCancelTaker(b *testing.B) {
	mid := int64(10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// build a book where every order is owned by user 42
		ob := models.NewOrderbook("BTC_USDT")
		var nextID int64 = 1
		for lvl := 1; lvl <= 10; lvl++ {
			for k := 0; k < 10; k++ {
				ob.Add(models.Order{
					ID: nextID, Symbol: "BTC_USDT", UserID: 42, Type: models.LIMIT,
					Side: models.SELL, Price: p(mid + int64(lvl)), Volume: q(1),
				})
				nextID++
			}
		}
		taker := models.Order{
			ID: int64(i + 1_000_000), Symbol: "BTC_USDT", UserID: 42,
			Type: models.LIMIT, Side: models.BUY,
			STP:   models.STPCancelTaker,
			Price: p(mid + 5), Volume: q(1),
		}
		b.StartTimer()

		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkWithL2Sink measures the cost of synchronous L2 emission. The
// sink is a no-op closure to isolate the emit-and-call overhead from
// whatever a real subscriber would do.
func BenchmarkWithL2Sink(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(1_000, 100, mid)
	ob.SetL2Sink(func(models.BookDelta) {})

	taker := models.Order{
		Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price:  p(mid - 50),
		Volume: q(1),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taker.ID = int64(i + 1)
		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkEndToEndMixed_Phase3 mirrors BenchmarkEndToEndMixed but exercises
// the Phase 3 surface: every order carries UserID + STP + TIF defaults, and
// an L2 sink is installed. This is the "production-shape" workload number
// the README should publish.
func BenchmarkEndToEndMixed_Phase3(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(2000, 200, mid)
	ob.SetL2Sink(func(models.BookDelta) {})
	r := rand.New(rand.NewPCG(42, 1024))

	var nextID int64 = 10_000_000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nextID++
		roll := r.Float64()
		side := models.BUY
		if r.Float64() < 0.5 {
			side = models.SELL
		}

		switch {
		case roll < 0.70:
			offset := int64(2 + r.IntN(50))
			price := p(mid - offset)
			if side == models.SELL {
				price = p(mid + offset)
			}
			vol := models.Qty(5000_0000 + r.Int64N(1_0000_0000))
			_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
				ID: nextID, Symbol: "BTC_USDT", UserID: nextID & 0xFF, // 256 distinct users
				Type:  models.LIMIT,
				Side:  side,
				STP:   models.STPCancelTaker,
				Price: price, Volume: vol,
			})
		case roll < 0.90:
			price := p(mid + 5)
			if side == models.SELL {
				price = p(mid - 5)
			}
			vol := models.Qty(5000_0000 + r.Int64N(1_0000_0000))
			_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
				ID: nextID, Symbol: "BTC_USDT", UserID: nextID & 0xFF,
				Type:  models.LIMIT,
				Side:  side,
				STP:   models.STPCancelTaker,
				Price: price, Volume: vol,
			})
		default:
			book := ob.Buy
			if side == models.SELL {
				book = ob.Sell
			}
			if len(book) > 0 && book[0].Orders.Head() != nil {
				_ = matchEngine.CancelOrder(ob, book[0].Orders.Head().Order.ID)
			}
		}
	}
}
