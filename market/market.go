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
	"time"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/metrics"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	"github.com/aliraad79/Gun/tracing"
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

// Options configure a Market's behavior. Zero values are sensible defaults
// for everything except Journal, which is mandatory: leaving it nil is a
// programmer error because it means accepted orders are NOT durable and a
// crash will silently lose state. Tests and benchmarks that genuinely don't
// want durability must opt into that explicitly via journal.Discard{}.
type Options struct {
	InboxSize int       // channel buffer; default 1024
	OnMatch   MatchSink
	OnBook    BookSink
	OnReject  RejectSink
	OnL2      models.L2Sink   // Level-2 (aggregated by price) order-book updates
	Journal   journal.Journal // REQUIRED: write-ahead log for crash recovery
	Persist   bool            // when true, snapshot to Redis after every op
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

	// Crash recovery: replay the journal *before* installing the L2
	// callback so replay does not flood subscribers with synthetic
	// events. The book ends up in the same state it was at the moment
	// the last journal record was fsynced.
	if err := opts.Journal.Replay(symbol, func(rec journal.Record) error {
		applyJournalRecord(book, rec)
		return nil
	}); err != nil {
		log.Error("journal replay failed for ", symbol, ": ", err)
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

// applyJournalRecord re-runs one journaled op against the book during
// crash recovery. Rejected ops are silently re-rejected — the journal
// includes them by design so that replay is bit-identical to the original
// processing path.
func applyJournalRecord(book *models.Orderbook, rec journal.Record) {
	switch rec.Kind {
	case journal.RecNew:
		_ = matchEngine.MatchAndAddNewOrder(book, rec.Order)
	case journal.RecCancel:
		_ = matchEngine.CancelOrder(book, rec.OrderID)
	case journal.RecModify:
		_ = matchEngine.ModifyOrder(book, rec.OrderID, rec.NewPrice, rec.NewVolume)
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
	start := time.Now()
	opName := "new"

	// One span per op. The sampler in tracing.Init decides whether to
	// actually record; either way the cost on the hot path is bounded
	// (no-op when disabled, ~1 µs when sampled).
	spanCtx, span := tracing.Start(context.Background(), "market.op",
		tracing.StringAttr("symbol", m.Symbol),
		tracing.Int64Attr("order_id", o.order.ID),
	)
	_ = spanCtx
	defer func() {
		span.SetAttributes(tracing.StringAttr("op", opName))
		span.End()
	}()

	switch o.kind {
	case opNewOrder:
		if err := models.Validate(o.order); err != nil {
			log.Warn("invalid order dropped: ", o.order)
			metrics.Order(m.Symbol, "rejected_"+matchEngine.RejectInvalidOrder)
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, matchEngine.RejectInvalidOrder)
			}
			return
		}
		// Write-ahead: durably record the op before applying. A crash
		// between Append and the engine call replays cleanly; a crash
		// before Append loses the op entirely, which is the correct
		// behavior (the producer is responsible for retry on timeout).
		jStart := time.Now()
		if err := m.opts.Journal.Append(m.Symbol, journal.Record{
			Kind: journal.RecNew, Order: o.order,
		}); err != nil {
			log.Error("journal append failed; dropping order ", o.order.ID, ": ", err)
			metrics.Order(m.Symbol, "rejected_journal_append_failed")
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, "journal_append_failed")
			}
			return
		}
		metrics.JournalAppendDuration(time.Since(jStart))

		res := matchEngine.MatchAndAddNewOrder(m.book, o.order)
		if !res.Accepted {
			metrics.Order(m.Symbol, "rejected_"+res.Reason)
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, res.Reason)
			}
			return
		}
		metrics.Order(m.Symbol, "accepted")
		matches := res.Matches
		if len(matches) > 0 {
			matches = append(matches,
				matchEngine.HandleConditionalOrders(m.book, matches)...)
			metrics.Matches(m.Symbol, len(matches))
			if m.opts.OnMatch != nil {
				m.opts.OnMatch(m.Symbol, matches)
			}
		}

	case opCancel:
		opName = "cancel"
		jStart := time.Now()
		if err := m.opts.Journal.Append(m.Symbol, journal.Record{
			Kind: journal.RecCancel, OrderID: o.order.ID, Symbol: m.Symbol,
		}); err != nil {
			log.Error("journal append failed; dropping cancel ", o.order.ID, ": ", err)
			return
		}
		metrics.JournalAppendDuration(time.Since(jStart))
		_ = matchEngine.CancelOrder(m.book, o.order.ID)

	case opModify:
		opName = "modify"
		jStart := time.Now()
		if err := m.opts.Journal.Append(m.Symbol, journal.Record{
			Kind: journal.RecModify, OrderID: o.order.ID, Symbol: m.Symbol,
			NewPrice: o.newPrice, NewVolume: o.newVolume,
		}); err != nil {
			log.Error("journal append failed; dropping modify ", o.order.ID, ": ", err)
			return
		}
		metrics.JournalAppendDuration(time.Since(jStart))
		res := matchEngine.ModifyOrder(m.book, o.order.ID, o.newPrice, o.newVolume)
		if !res.Accepted {
			if m.opts.OnReject != nil {
				m.opts.OnReject(m.Symbol, o.order, res.Reason)
			}
			break
		}
		if len(res.Matches) > 0 {
			metrics.Matches(m.Symbol, len(res.Matches))
			if m.opts.OnMatch != nil {
				m.opts.OnMatch(m.Symbol, res.Matches)
			}
		}
	}

	if m.opts.Persist {
		persistance.CommitOrderBook(m.book)
	}
	if m.opts.OnBook != nil {
		m.opts.OnBook(m.book)
	}

	metrics.OpDuration(m.Symbol, opName, time.Since(start))
	metrics.BookDepth(m.Symbol,
		m.book.LevelCount(models.BUY), m.book.OrderCount(models.BUY),
		m.book.LevelCount(models.SELL), m.book.OrderCount(models.SELL),
	)
}

// Book returns the Market's orderbook. The returned pointer is only safe
// to read from the same goroutine that calls Run, or after that goroutine
// has exited. Tests use this for assertions after Shutdown.
func (m *Market) Book() *models.Orderbook { return m.book }
