package market

import (
	"context"
	"sync"

	"github.com/aliraad79/Gun/models"
)

// Registry lazily creates one Market goroutine per symbol on first message.
// All routing decisions live here; Markets themselves are self-contained.
//
// Registry is safe for concurrent use across producer goroutines.
type Registry struct {
	ctx  context.Context
	wg   *sync.WaitGroup
	opts Options

	mu      sync.Mutex
	markets map[string]*Market
}

// NewRegistry returns a Registry that will spawn Market goroutines as a
// child of ctx and add them to wg. Cancel ctx to shut all markets down,
// then call wg.Wait() to block until they exit.
//
// opts is applied to every Market created by this Registry; in particular,
// OnMatch / OnBook callbacks defined here are invoked by every market.
//
// Journal is REQUIRED. A nil journal means accepted orders are not durable
// and a crash will silently lose state, which is never what a production
// caller wants. If you genuinely want to run without durability (tests,
// benchmarks, throwaway demos), pass &journal.Discard{} explicitly so the
// intent is visible in the code.
func NewRegistry(ctx context.Context, wg *sync.WaitGroup, opts Options) *Registry {
	if opts.Journal == nil {
		panic("market: Options.Journal is required; pass &journal.Discard{} to opt out of durability explicitly")
	}
	return &Registry{
		ctx:     ctx,
		wg:      wg,
		opts:    opts,
		markets: make(map[string]*Market),
	}
}

// Submit routes an order to the right market. The market is created on
// first contact for a given symbol.
func (r *Registry) Submit(order models.Order) {
	m := r.getOrCreate(order.Symbol)
	m.Submit(order)
}

// Cancel routes a cancel by orderID to the right market. order.Symbol must
// identify the market; order.ID is the target.
func (r *Registry) Cancel(order models.Order) {
	m := r.getOrCreate(order.Symbol)
	m.Cancel(order.ID)
}

// Modify routes an order modification to the right market. See
// matchEngine.ModifyOrder for price/quantity semantics.
func (r *Registry) Modify(symbol string, orderID int64, newPrice models.Px, newVolume models.Qty) {
	m := r.getOrCreate(symbol)
	m.Modify(orderID, newPrice, newVolume)
}

// Get returns the existing market for a symbol, creating it if necessary.
// Exposed for tests that want to introspect a market.
func (r *Registry) Get(symbol string) *Market {
	return r.getOrCreate(symbol)
}

// Count returns the number of markets currently active. Exposed for tests
// and observability.
func (r *Registry) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.markets)
}

func (r *Registry) getOrCreate(symbol string) *Market {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m, ok := r.markets[symbol]; ok {
		return m
	}

	m := newMarket(symbol, r.opts)
	r.markets[symbol] = m
	r.wg.Add(1)
	go m.run(r.ctx, r.wg)
	return m
}
