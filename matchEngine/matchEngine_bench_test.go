package matchEngine_test

import (
	"math/rand/v2"
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/shopspring/decimal"
)

// buildBook seeds a book with depthPerSide orders on each side, spread across
// numLevels price levels symmetric around midPrice. Returns a fresh orderbook
// ready for benchmarking.
func buildBook(depthPerSide, numLevels int, midPrice int64) *models.Orderbook {
	ob := &models.Orderbook{Symbol: "BTC_USDT"}
	r := rand.New(rand.NewPCG(1, 2))

	var nextID int64 = 1
	ordersPerLevel := depthPerSide / numLevels
	if ordersPerLevel < 1 {
		ordersPerLevel = 1
	}

	for lvl := 1; lvl <= numLevels; lvl++ {
		buyPrice := decimal.NewFromInt(midPrice - int64(lvl))
		sellPrice := decimal.NewFromInt(midPrice + int64(lvl))

		for i := 0; i < ordersPerLevel; i++ {
			vol := decimal.NewFromFloat(1 + r.Float64())

			ob.Add(models.Order{
				ID: nextID, Symbol: "BTC_USDT", Type: models.LIMIT,
				Side: models.BUY, Price: buyPrice, Volume: vol,
			})
			nextID++
			ob.Add(models.Order{
				ID: nextID, Symbol: "BTC_USDT", Type: models.LIMIT,
				Side: models.SELL, Price: sellPrice, Volume: vol,
			})
			nextID++
		}
	}
	return ob
}

// BenchmarkAddNonCrossing measures the cost of inserting a limit order that
// does NOT cross the spread (just rests on the book). Hot path for posting
// liquidity. Depth = 1000 levels per side, taker priced one tick beyond best.
func BenchmarkAddNonCrossing(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(10_000, 1000, mid)

	taker := models.Order{
		ID: 0, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
		Price:  decimal.NewFromInt(mid - 2000), // far below best bid
		Volume: decimal.NewFromInt(1),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		taker.ID = int64(i + 1)
		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkMatchAtBest measures the dominant case: a taker that fully
// consumes a single resting order at best price.
func BenchmarkMatchAtBest(b *testing.B) {
	mid := int64(10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ob := buildBook(100, 10, mid)
		taker := models.Order{
			ID: int64(i + 1_000_000), Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
			Price: decimal.NewFromInt(mid + 1), Volume: decimal.NewFromFloat(0.1),
		}
		b.StartTimer()

		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkSweepFiveLevels measures a multi-level sweep: taker consumes
// liquidity across 5 price levels in one go.
func BenchmarkSweepFiveLevels(b *testing.B) {
	mid := int64(10_000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		ob := buildBook(500, 10, mid)
		taker := models.Order{
			ID: int64(i + 1_000_000), Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY,
			Price: decimal.NewFromInt(mid + 5), Volume: decimal.NewFromInt(100),
		}
		b.StartTimer()

		_ = matchEngine.MatchAndAddNewOrder(ob, taker)
	}
}

// BenchmarkCancelMidBook measures order cancellation at a non-best price level.
// Worst case for the current O(n*m) cancel: target sits in the middle of the
// book.
func BenchmarkCancelMidBook(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(2000, 200, mid)

	// Pick an order that exists somewhere in the middle of the book.
	var targetID int64
	if len(ob.Buy) > 100 && len(ob.Buy[100].Orders) > 0 {
		targetID = ob.Buy[100].Orders[0].ID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchEngine.CancelOrder(ob, targetID) // most iterations will miss after first
	}
}

// BenchmarkEndToEndMixed simulates a realistic order flow: 70% post (resting),
// 20% take (crosses), 10% cancel. Drives the full MatchAndAddNewOrder /
// CancelOrder surface.
func BenchmarkEndToEndMixed(b *testing.B) {
	mid := int64(10_000)
	ob := buildBook(2000, 200, mid)
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
		case roll < 0.70: // post liquidity away from best
			offset := int64(2 + r.IntN(50))
			price := mid - offset
			if side == models.SELL {
				price = mid + offset
			}
			_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
				ID: nextID, Symbol: "BTC_USDT", Type: models.LIMIT, Side: side,
				Price: decimal.NewFromInt(price), Volume: decimal.NewFromFloat(0.5 + r.Float64()),
			})
		case roll < 0.90: // taker that crosses
			price := mid + 5
			if side == models.SELL {
				price = mid - 5
			}
			_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
				ID: nextID, Symbol: "BTC_USDT", Type: models.LIMIT, Side: side,
				Price: decimal.NewFromInt(price), Volume: decimal.NewFromFloat(0.5 + r.Float64()),
			})
		default: // cancel a random resting order if any exist
			book := ob.Buy
			if side == models.SELL {
				book = ob.Sell
			}
			if len(book) > 0 && len(book[0].Orders) > 0 {
				_ = matchEngine.CancelOrder(ob, book[0].Orders[0].ID)
			}
		}
	}
}
