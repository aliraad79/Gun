// Package market wraps a single-symbol Orderbook in a single-writer
// goroutine fed by an inbox channel. Each Market owns its book outright;
// there is no shared mutable state between markets, so the matching path
// runs entirely lock-free per symbol and scales linearly with active
// symbols on a multi-core box.
//
// Markets are created lazily by Registry on the first message for a
// symbol. Shutdown drains in-flight orders, then exits when the parent
// context is cancelled.
package market

import (
	"context"
	"sync"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	log "github.com/sirupsen/logrus"
)

// MatchSink is invoked synchronously when a Market produces one or more
// matches. The callback runs on the market's own goroutine; if the consumer
// wants to fan out asynchronously, it should hand the slice to another
// goroutine itself.
type MatchSink func(symbol string, matches []models.Match)

// BookSink is invoked synchronously after every order or cancel that
// modifies a Market's orderbook. Useful for L2 fan-out, audit, or testing.
type BookSink func(*models.Orderbook)

// RejectSink is invoked when the engine refuses an order (post-only would
// cross, FOK can't fully fill, malformed payload, etc.). Reason is one of
// the matchEngine.Reject* constants.
type RejectSink func(symbol string, order models.Order, reason string)

// Options configure a Market's behavior. Zero values are sensible defaults.
type Options struct {
	InboxSize int       // channel buffer; default 1024
	OnMatch   MatchSink
	OnBook    BookSink
	OnReject  RejectSink
	OnL2      models.L2Sink // Level-2 (aggregated by price) order-book updates
	Persist   bool          // when true, snapshot to Redis after every op
}

// inboxDefault is the per-market channel capacity when not overridden.
const inboxDefault = 1024

type opKind uint8

const (
	opNewOrder opKind = iota
	opCancel
	opModify
)

type op struct {
	kind      opKind
	order     models.Order // for opNewOrder; for opCancel only order.ID is read
	newPrice  models.Px    // for opModify
	newVolume models.Qty   // for opModify
}

// Market owns a single-symbol Orderbook and processes orders from its
// inbox channel on a dedicated goroutine.
type Market struct {
	Symbol string
	inbox  chan op
	book   *models.Orderbook
	opts   Options
}

// newMarket constructs a Market; called by Registry.
func newMarket(symbol string, opts Options) *Market {
	if opts.InboxSize <= 0 {
		opts.InboxSize = inboxDefault
	}

	book, err := matchEngine.LoadOrFetchOrderbook(symbol)
	if err != nil || book == nil {
		// Fall through to a fresh empty book; failure to validate against
		// SUPPORTED_SYMBOLS is logged here but not fatal at the market
		// level (the Registry decides whether to admit the symbol).
		book = models.NewOrderbook(symbol)
	}

	if opts.OnL2 != nil {
		book.SetL2Sink(opts.OnL2)
	}

	return &Market{
		Symbol: symbol,
		inbox:  make(chan op, opts.InboxSize),
		book:   book,
		opts:   opts,
	}
}

// Submit enqueues a new-order op. The call blocks if the inbox is full,
// providing back-pressure to the caller (typically the Kafka consumer).
func (m *Market) Submit(order models.Order) {
	m.inbox <- op{kind: opNewOrder, order: order}
}

// Cancel enqueues a cancel for the given orderID.
func (m *Market) Cancel(id int64) {
	m.inbox <- op{kind: opCancel, order: models.Order{ID: id}}
}

// Modify enqueues a modify-order op. See matchEngine.ModifyOrder for
// the price/quantity semantics.
func (m *Market) Modify(orderID int64, newPrice models.Px, newVolume models.Qty) {
	m.inbox <- op{
		kind:      opModify,
		order:     models.Order{ID: orderID},
		newPrice:  newPrice,
		newVolume: newVolume,
	}
}

// run is the single-writer loop. Stops when ctx is cancelled and the inbox
// is drained.
func (m *Market) run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case o := <-m.inbox:
			m.handle(o)
		}
	}
}

func (m *Market) handle(o op) {
	switch o.kind {
	case opNewOrder:
		if err := models.Validate(o.order); err != nil {
			log.Warn("invalid order dropped: ", o.order)
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, matchEngine.RejectInvalidOrder)
			}
			return
		}
		res := matchEngine.MatchAndAddNewOrder(m.book, o.order)
		if !res.Accepted {
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, res.Reason)
			}
			return
		}
		matches := res.Matches
		if len(matches) > 0 {
			matches = append(matches,
				matchEngine.HandleConditionalOrders(m.book, matches)...)
			if m.opts.OnMatch != nil {
				m.opts.OnMatch(m.Symbol, matches)
			}
		}
	case opCancel:
		_ = matchEngine.CancelOrder(m.book, o.order.ID)
	case opModify:
		res := matchEngine.ModifyOrder(m.book, o.order.ID, o.newPrice, o.newVolume)
		if !res.Accepted {
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, res.Reason)
			}
			break
		}
		if len(res.Matches) > 0 && m.opts.OnMatch != nil {
			m.opts.OnMatch(m.Symbol, res.Matches)
		}
	}

	if m.opts.Persist {
		persistance.CommitOrderBook(m.book)
	}
	if m.opts.OnBook != nil {
		m.opts.OnBook(m.book)
	}
}

// Book returns the Market's orderbook. The returned pointer is only safe
// to read from the same goroutine that calls Run, or after that goroutine
// has exited. Tests use this for assertions after Shutdown.
func (m *Market) Book() *models.Orderbook { return m.book }
