package market_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/market"
	"github.com/aliraad79/Gun/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// End-to-end: drive a Market with a real FileJournal, shut it down,
// start a fresh Market against the same journal, and verify it ends up
// in the same book state.
func TestJournal_CrashRecoveryReproducesBook(t *testing.T) {
	t.Setenv("SUPPORTED_SYMBOLS", "BTC_USDT")

	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })

	// ---- run 1: post some liquidity, cross some, cancel some ----
	{
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		r := market.NewRegistry(ctx, &wg, market.Options{
			InboxSize: 64,
			Journal:   j,
		})

		r.Submit(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(101), Volume: q(2)})
		r.Submit(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(102), Volume: q(3)})
		r.Submit(models.Order{ID: 3, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(99), Volume: q(5)})
		// crossing buy: takes the 101 fully (2), leaves taker remainder 3 to rest at 101
		r.Submit(models.Order{ID: 4, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(101), Volume: q(5)})
		// cancel the 99 buy
		r.Cancel(models.Order{ID: 3, Symbol: "BTC_USDT"})

		// drain inbox, then shut down
		time.Sleep(80 * time.Millisecond)
		cancel()
		wg.Wait()
	}

	// At this point the journal on disk holds 5 records. The original
	// engine instance is gone (registry shut down). Now build a fresh
	// one against the SAME journal and confirm it replays cleanly.

	// ---- run 2: fresh engine, journal-only recovery ----
	{
		// open a new journal handle pointing at the same dir, simulating
		// a fresh process startup.
		j2, err := journal.NewFileJournal(dir, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = j2.Close() })

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		r := market.NewRegistry(ctx, &wg, market.Options{
			InboxSize: 64,
			Journal:   j2,
		})

		// Touch the market via a no-op cancel of a non-existent order.
		// This forces lazy creation of the BTC_USDT Market, which in turn
		// triggers Replay during newMarket().
		r.Cancel(models.Order{ID: 999_999, Symbol: "BTC_USDT"})
		time.Sleep(50 * time.Millisecond)

		// Inspect the recovered book.
		book := r.Get("BTC_USDT").Book()

		// Expected state after replay:
		//   ask: 102 / qty 3  (the 2-at-101 was fully consumed)
		//   bid: 101 / qty 3  (the buy of 5 took 2 and rested 3)
		//   (the 99 buy was cancelled)
		if assert.Len(t, book.Sell, 1) {
			assert.Equal(t, p(102), book.Sell[0].Price)
			assert.Equal(t, q(3), book.Sell[0].TotalQty())
		}
		if assert.Len(t, book.Buy, 1) {
			assert.Equal(t, p(101), book.Buy[0].Price)
			assert.Equal(t, q(3), book.Buy[0].TotalQty())
		}

		cancel()
		wg.Wait()
	}
}

// Replaying a journal should not flood any L2 subscriber that gets
// installed AFTER replay (since they only care about live ops, not
// historical reconstruction).
func TestJournal_ReplayDoesNotFloodL2Subscribers(t *testing.T) {
	t.Setenv("SUPPORTED_SYMBOLS", "BTC_USDT")

	dir := t.TempDir()
	j, err := journal.NewFileJournal(dir, false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = j.Close() })

	// seed the journal with a few ops
	{
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		r := market.NewRegistry(ctx, &wg, market.Options{Journal: j})
		r.Submit(models.Order{ID: 1, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.SELL, Price: p(100), Volume: q(1)})
		r.Submit(models.Order{ID: 2, Symbol: "BTC_USDT", Type: models.LIMIT, Side: models.BUY, Price: p(99), Volume: q(1)})
		time.Sleep(40 * time.Millisecond)
		cancel()
		wg.Wait()
	}

	// recover with an L2 sink installed. The sink should NOT see any of
	// the seed deltas (they belong to the previous run).
	{
		var deltasMu sync.Mutex
		var deltas []models.BookDelta

		j2, err := journal.NewFileJournal(dir, false)
		require.NoError(t, err)
		t.Cleanup(func() { _ = j2.Close() })

		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		r := market.NewRegistry(ctx, &wg, market.Options{
			Journal: j2,
			OnL2: func(d models.BookDelta) {
				deltasMu.Lock()
				deltas = append(deltas, d)
				deltasMu.Unlock()
			},
		})

		// force market creation; replay happens inside newMarket *before*
		// the L2 sink is installed, so no deltas should fire.
		r.Cancel(models.Order{ID: 999, Symbol: "BTC_USDT"})
		time.Sleep(40 * time.Millisecond)

		deltasMu.Lock()
		assert.Empty(t, deltas, "replay must not flood newly-subscribed L2 listeners")
		deltasMu.Unlock()

		cancel()
		wg.Wait()
	}
}
