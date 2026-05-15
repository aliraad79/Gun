package market_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aliraad79/Gun/market"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
)

func p(v int64) models.Px  { return models.Px(v * 1_0000_0000) }
func q(v int64) models.Qty { return models.Qty(v * 1_0000_0000) }

// drainRegistry runs fn against a Registry created for the duration of the
// test. The Registry is shut down and all its Market goroutines are joined
// before fn returns, so post-condition assertions see a quiescent system.
func drainRegistry(t *testing.T, opts market.Options, fn func(*market.Registry)) {
	t.Helper()
	t.Setenv("SUPPORTED_SYMBOLS", "BTC_USDT,ETH_USDT,DOGE_USDT")

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	r := market.NewRegistry(ctx, &wg, opts)

	fn(r)

	// Wait a beat so any in-flight ops drain through the channel before we
	// cancel. In a real run main.go has its own back-pressure from Kafka.
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
}

// Two symbols, two markets, two goroutines: orders sent to one symbol must
// not block orders to the other. We measure isolation by checking that
// matches recorded under each symbol equal what we sent.
func TestRegistry_CrossSymbolIsolation(t *testing.T) {
	var (
		btcMatches int64
		ethMatches int64
	)

	opts := market.Options{
		InboxSize: 256,
		OnMatch: func(symbol string, matches []models.Match) {
			switch symbol {
			case "BTC_USDT":
				atomic.AddInt64(&btcMatches, int64(len(matches)))
			case "ETH_USDT":
				atomic.AddInt64(&ethMatches, int64(len(matches)))
			}
		},
	}

	drainRegistry(t, opts, func(r *market.Registry) {
		// Seed each market with a resting sell, then send a crossing buy.
		r.Submit(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(10000), Volume: q(1)})
		r.Submit(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(10000), Volume: q(1)})

		r.Submit(models.Order{ID: 100, Symbol: "ETH_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(2000), Volume: q(5)})
		r.Submit(models.Order{ID: 101, Symbol: "ETH_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(2000), Volume: q(5)})
	})

	assert.Equal(t, int64(1), atomic.LoadInt64(&btcMatches), "BTC trades should match exactly once")
	assert.Equal(t, int64(1), atomic.LoadInt64(&ethMatches), "ETH trades should match exactly once")
}

// Orders submitted to the same market are processed in submission order.
// We verify this by chaining three orders such that the second crosses the
// first and the third crosses the residual from the second.
func TestMarket_ProcessesInSubmissionOrder(t *testing.T) {
	var (
		matchesMu sync.Mutex
		seen      []models.Match
	)

	opts := market.Options{
		InboxSize: 16,
		OnMatch: func(symbol string, matches []models.Match) {
			matchesMu.Lock()
			seen = append(seen, matches...)
			matchesMu.Unlock()
		},
	}

	drainRegistry(t, opts, func(r *market.Registry) {
		// rest a sell of 5 at 100
		r.Submit(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(5)})
		// buy 2 at 100 -> match of 2 against the sell, sell residual = 3
		r.Submit(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(2)})
		// buy 4 at 100 -> match of 3 (clears the level), residual buy of 1 rests
		r.Submit(models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(4)})
	})

	matchesMu.Lock()
	defer matchesMu.Unlock()

	if assert.Len(t, seen, 2) {
		assert.Equal(t, int64(2), seen[0].BuyId, "first match is order #2")
		assert.Equal(t, q(2), seen[0].Volume)

		assert.Equal(t, int64(3), seen[1].BuyId, "second match is order #3")
		assert.Equal(t, q(3), seen[1].Volume, "second match consumes the rest of the resting sell")
	}
}

// Cancel removes a resting order before it can be matched against.
func TestMarket_CancelBeforeMatch(t *testing.T) {
	var matchedCount int64
	opts := market.Options{
		InboxSize: 16,
		OnMatch: func(_ string, m []models.Match) {
			atomic.AddInt64(&matchedCount, int64(len(m)))
		},
	}

	drainRegistry(t, opts, func(r *market.Registry) {
		r.Submit(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)})
		// cancel it before the buyer arrives
		r.Cancel(models.Order{ID: 1, Symbol: "BTC_USDT"})
		// buyer should see an empty book
		r.Submit(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(100), Volume: q(1)})
	})

	assert.Equal(t, int64(0), atomic.LoadInt64(&matchedCount),
		"no match should occur — the resting sell was cancelled")
}

// Concurrent submissions from many producers to the same Registry must
// not race. Run with go test -race.
func TestRegistry_ConcurrentProducers(t *testing.T) {
	opts := market.Options{InboxSize: 1024}

	t.Setenv("SUPPORTED_SYMBOLS", "BTC_USDT,ETH_USDT")
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	r := market.NewRegistry(ctx, &wg, opts)

	const producers = 8
	const perProducer = 200

	var pwg sync.WaitGroup
	pwg.Add(producers)
	for prod := 0; prod < producers; prod++ {
		go func(prodID int) {
			defer pwg.Done()
			for i := 0; i < perProducer; i++ {
				symbol := "BTC_USDT"
				if i%2 == 0 {
					symbol = "ETH_USDT"
				}
				r.Submit(models.Order{
					ID:     int64(prodID*perProducer + i + 1),
					Symbol: symbol,
					Type:   models.LIMIT,
					Side:   models.BUY,
					Price:  p(1_000_000 - int64(i%100)),
					Volume: q(1),
				})
			}
		}(prod)
	}
	pwg.Wait()

	time.Sleep(100 * time.Millisecond)
	cancel()
	wg.Wait()

	assert.Equal(t, 2, r.Count(), "two distinct symbols should produce two markets")
}
