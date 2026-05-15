package models

import (
	"errors"
	"sort"

	protoModels "github.com/aliraad79/Gun/models/models"
	log "github.com/sirupsen/logrus"
)

// MatchEngineEntry is a single price level in the order book. The Orders
// list is FIFO (head = oldest); the level is removed from the book when
// the list empties.
//
// totalQty is a cached sum of every order's volume at this level. It is
// maintained by every code path that mutates the list — addOrder,
// removeOrder, reduceHead — so consumers can read it in O(1) without
// walking the linked list.
type MatchEngineEntry struct {
	Price    Px
	Orders   OrderList
	totalQty Qty
}

// TotalQty returns the aggregate resting volume at this level.
func (e *MatchEngineEntry) TotalQty() Qty { return e.totalQty }

// addOrder appends to the FIFO tail and updates totalQty.
func (e *MatchEngineEntry) addOrder(o Order) *OrderNode {
	n := e.Orders.PushBack(o, e)
	e.totalQty = e.totalQty.Add(o.Volume)
	return n
}

// removeNode unlinks n from the level and updates totalQty by the node's
// (current) volume.
func (e *MatchEngineEntry) removeNode(n *OrderNode) {
	e.totalQty = e.totalQty.Sub(n.Order.Volume)
	e.Orders.Remove(n)
}

// reduceHead reduces the head order's volume by `by` and updates
// totalQty accordingly. Caller must have already established that
// `by` <= head's volume.
func (e *MatchEngineEntry) reduceHead(by Qty) {
	head := e.Orders.Head()
	head.Order.Volume = head.Order.Volume.Sub(by)
	e.totalQty = e.totalQty.Sub(by)
}

// reduceNode reduces an arbitrary node's volume to newVolume and updates
// totalQty by the delta.
func (e *MatchEngineEntry) reduceNode(n *OrderNode, newVolume Qty) {
	delta := n.Order.Volume.Sub(newVolume)
	n.Order.Volume = newVolume
	e.totalQty = e.totalQty.Sub(delta)
}

// Orderbook holds bid and ask ladders for a single symbol, plus indexes
// for O(1) order lookup and O(1) price-level lookup.
//
// Internal invariants (single-writer; one Orderbook per goroutine):
//
//   - Buy is sorted strictly descending by Price (best bid at index 0).
//   - Sell is sorted strictly ascending by Price (best ask at index 0).
//   - bidByPrice / askByPrice contain exactly one *MatchEngineEntry per
//     price level present in Buy / Sell, pointing to the same struct as
//     the corresponding slice entry.
//   - orderIndex contains exactly one *OrderNode per resting order on
//     either side. The node's Level back-reference always points to
//     the level currently holding it.
type Orderbook struct {
	Symbol            string
	Buy               []*MatchEngineEntry
	Sell              []*MatchEngineEntry
	ConditionalOrders []Order

	// seq is the next sequence number to hand out to a Match. Strictly
	// monotonic per-symbol; persisted into snapshots so recovery resumes
	// at the next un-used value.
	seq uint64

	bidByPrice map[Px]*MatchEngineEntry
	askByPrice map[Px]*MatchEngineEntry
	orderIndex map[int64]*OrderNode

	// l2 is the optional Level-2 publishing callback. When set, every
	// price-level qty change emits a BookDelta with a fresh seq.
	l2 L2Sink
}

// SetL2Sink installs an L2 publishing callback. Pass nil to disable.
// Safe to call before any orders have been routed into the book; not
// safe to call concurrently with matching (use this at construction).
func (ob *Orderbook) SetL2Sink(sink L2Sink) {
	ob.l2 = sink
}

// emitDelta fires the L2 callback (if installed) with a fresh seq and
// the current aggregate qty at the level. Qty == 0 means the level
// no longer exists.
func (ob *Orderbook) emitDelta(side Side, price Px, qty Qty) {
	if ob.l2 == nil {
		return
	}
	ob.l2(BookDelta{
		Seq:    ob.NextSeq(),
		Symbol: ob.Symbol,
		Side:   side,
		Price:  price,
		Qty:    qty,
	})
}

// NewOrderbook returns an Orderbook with its internal indexes initialized.
// Use this constructor everywhere; a zero-value Orderbook will not work
// (the maps are nil).
func NewOrderbook(symbol string) *Orderbook {
	return &Orderbook{
		Symbol:     symbol,
		bidByPrice: make(map[Px]*MatchEngineEntry),
		askByPrice: make(map[Px]*MatchEngineEntry),
		orderIndex: make(map[int64]*OrderNode),
	}
}

// ensureMaps lazily initializes the indexes for orderbooks constructed
// via struct literal (test fixtures, protobuf round-trip). New code should
// prefer NewOrderbook.
func (ob *Orderbook) ensureMaps() {
	if ob.bidByPrice == nil {
		ob.bidByPrice = make(map[Px]*MatchEngineEntry)
	}
	if ob.askByPrice == nil {
		ob.askByPrice = make(map[Px]*MatchEngineEntry)
	}
	if ob.orderIndex == nil {
		ob.orderIndex = make(map[int64]*OrderNode)
	}
}

// Add places order on its side of the book and registers it in the orderID
// index. If a level at this price already exists, the order is appended to
// the FIFO tail. Otherwise a new level is inserted at the correct sorted
// position by binary search (O(log n) on the ladder depth).
//
// Returns the node so callers can hold the reference if they want; most
// callers ignore it.
func (ob *Orderbook) Add(order Order) *OrderNode {
	ob.ensureMaps()

	switch order.Side {
	case BUY:
		return ob.addSide(&ob.Buy, ob.bidByPrice, order, true)
	case SELL:
		return ob.addSide(&ob.Sell, ob.askByPrice, order, false)
	default:
		log.Error("unexpected order.Side: ", order.Side)
		return nil
	}
}

// addSide is the shared implementation for the two ladder sides. descending
// is true for the bid ladder (best price = highest), false for the ask
// ladder (best price = lowest).
func (ob *Orderbook) addSide(book *[]*MatchEngineEntry, byPrice map[Px]*MatchEngineEntry, order Order, descending bool) *OrderNode {
	if level, ok := byPrice[order.Price]; ok {
		node := level.addOrder(order)
		ob.orderIndex[order.ID] = node
		ob.emitDelta(order.Side, level.Price, level.totalQty)
		return node
	}

	level := &MatchEngineEntry{Price: order.Price}
	node := level.addOrder(order)
	byPrice[order.Price] = level
	ob.orderIndex[order.ID] = node

	// Find insertion index by binary search: maintain the side-specific sort.
	idx := sort.Search(len(*book), func(i int) bool {
		if descending {
			return (*book)[i].Price.Lt(order.Price)
		}
		return (*book)[i].Price.Gt(order.Price)
	})
	*book = append(*book, nil)
	copy((*book)[idx+1:], (*book)[idx:])
	(*book)[idx] = level

	ob.emitDelta(order.Side, level.Price, level.totalQty)
	return node
}

// AddConditionalOrder enqueues a conditional (e.g. stop-limit) order for
// later re-evaluation against the running trade tape.
func (ob *Orderbook) AddConditionalOrder(order Order) {
	ob.ConditionalOrders = append(ob.ConditionalOrders, order)
}

// NextSeq returns the next match sequence number for this symbol and
// advances the counter. The single-writer-per-symbol invariant means no
// locking is needed.
func (ob *Orderbook) NextSeq() uint64 {
	ob.seq++
	return ob.seq
}

// Seq returns the current sequence value without advancing it. Exposed
// for snapshot writers and tests.
func (ob *Orderbook) Seq() uint64 { return ob.seq }

// SetSeq sets the next-to-be-used sequence number. Used by snapshot
// recovery to resume the counter where it left off.
func (ob *Orderbook) SetSeq(s uint64) { ob.seq = s }

// ErrCancelOrderFailed is returned by Cancel when the order ID is unknown.
var ErrCancelOrderFailed = errors.New("cancelling order failed")

// ErrModifyIncrease is returned by ReduceVolume when the new volume is
// not strictly smaller than the current resting volume.
var ErrModifyIncrease = errors.New("reduce: new volume must be smaller than resting")

// LookupOrder returns the resting order with the given ID, or false if
// no such order is resting. The returned Order is a copy; mutating it
// has no effect on the book.
func (ob *Orderbook) LookupOrder(orderID int64) (Order, bool) {
	ob.ensureMaps()
	n, ok := ob.orderIndex[orderID]
	if !ok {
		return Order{}, false
	}
	return n.Order, true
}

// ReduceVolume shrinks a resting order's quantity in place, preserving
// its FIFO queue position at the level. Returns ErrCancelOrderFailed if
// the order is not on the book; ErrModifyIncrease if the new volume is
// not strictly smaller than the current resting volume.
//
// A new volume of zero is equivalent to a cancel and is delegated to
// Cancel.
func (ob *Orderbook) ReduceVolume(orderID int64, newVolume Qty) error {
	ob.ensureMaps()
	if newVolume.IsZero() {
		return ob.Cancel(orderID)
	}
	n, ok := ob.orderIndex[orderID]
	if !ok {
		return ErrCancelOrderFailed
	}
	if !newVolume.Lt(n.Order.Volume) {
		return ErrModifyIncrease
	}
	level := n.Level
	level.reduceNode(n, newVolume)
	ob.emitDelta(n.Order.Side, level.Price, level.totalQty)
	return nil
}

// Cancel removes the order with id targetID from the book. O(1) order
// lookup via the orderID index; O(log n) on level removal if the level
// empties.
func (ob *Orderbook) Cancel(targetID int64) error {
	ob.ensureMaps()

	node, ok := ob.orderIndex[targetID]
	if !ok {
		return ErrCancelOrderFailed
	}
	level := node.Level
	side := node.Order.Side
	level.removeNode(node)
	delete(ob.orderIndex, targetID)

	if level.Orders.IsEmpty() {
		switch side {
		case BUY:
			ob.removeLevel(&ob.Buy, ob.bidByPrice, level, true)
		case SELL:
			ob.removeLevel(&ob.Sell, ob.askByPrice, level, false)
		}
	}
	ob.emitDelta(side, level.Price, level.totalQty)
	return nil
}

// removeLevel removes an emptied level from the ladder and the byPrice map.
func (ob *Orderbook) removeLevel(book *[]*MatchEngineEntry, byPrice map[Px]*MatchEngineEntry, level *MatchEngineEntry, descending bool) {
	delete(byPrice, level.Price)

	// Binary search for the level's index in the side-specific ordering.
	idx := sort.Search(len(*book), func(i int) bool {
		if descending {
			return (*book)[i].Price.Lte(level.Price)
		}
		return (*book)[i].Price.Gte(level.Price)
	})
	if idx >= len(*book) || (*book)[idx] != level {
		// invariant violation; shouldn't happen but don't corrupt state
		log.Error("removeLevel: level not found in book at expected index")
		return
	}
	copy((*book)[idx:], (*book)[idx+1:])
	(*book)[len(*book)-1] = nil
	*book = (*book)[:len(*book)-1]
}

// removeFrontLevel pops the best-priced (index 0) level off a side after
// it has been fully consumed by matching. O(n) memmove on the slice
// header but constant work on the map.
func (ob *Orderbook) removeFrontLevel(side Side) {
	switch side {
	case BUY:
		if len(ob.Buy) == 0 {
			return
		}
		delete(ob.bidByPrice, ob.Buy[0].Price)
		ob.Buy[0] = nil
		ob.Buy = ob.Buy[1:]
	case SELL:
		if len(ob.Sell) == 0 {
			return
		}
		delete(ob.askByPrice, ob.Sell[0].Price)
		ob.Sell[0] = nil
		ob.Sell = ob.Sell[1:]
	}
}

// MatchTaker walks the opposite-side ladder in price-time priority and
// fills as much of taker as the crossing levels allow. The book is mutated
// in place: resting orders are reduced or fully removed (and their entries
// in orderIndex deleted); price levels are removed when their FIFO list
// empties.
//
// alwaysCross=true bypasses the price-crossing check and is used for
// market orders.
//
// Self-trade prevention: when the taker carries a non-zero UserID and a
// non-empty STP mode, resting orders owned by the same user are handled
// per the taker's STP setting instead of producing a real trade match.
//
// Returns the produced matches and the unfilled remainder of taker.
func (ob *Orderbook) MatchTaker(taker Order, alwaysCross bool) ([]Match, Qty) {
	ob.ensureMaps()

	var oppositeSide Side
	switch taker.Side {
	case BUY:
		oppositeSide = SELL
	case SELL:
		oppositeSide = BUY
	default:
		return nil, taker.Volume
	}

	stpEnabled := taker.UserID != 0 && taker.STP != STPNone

	var matches []Match
	remain := taker.Volume

	for remain.IsPositive() {
		level := ob.frontLevel(oppositeSide)
		if level == nil {
			break
		}
		if !alwaysCross && !crosses(taker, level.Price) {
			break
		}

		levelChanged := false

		// price-time priority: oldest order at this level matches first
		for remain.IsPositive() {
			node := level.Orders.Head()
			if node == nil {
				break
			}
			resting := node.Order

			// Self-trade prevention: if same UserID, apply STP mode.
			if stpEnabled && resting.UserID == taker.UserID {
				before := level.totalQty
				done := ob.applySTP(taker.STP, &remain, node, level)
				if !level.totalQty.Eq(before) {
					levelChanged = true
				}
				if done {
					break
				}
				continue
			}

			fill := MinQty(remain, resting.Volume)

			m := makeMatch(taker, resting, level.Price, fill)
			m.Seq = ob.NextSeq()
			matches = append(matches, m)
			remain = remain.Sub(fill)

			if fill.Eq(resting.Volume) {
				level.removeNode(node)
				delete(ob.orderIndex, resting.ID)
			} else {
				level.reduceHead(fill)
			}
			levelChanged = true
		}

		levelEmpty := level.Orders.IsEmpty()
		levelPrice := level.Price
		if levelEmpty {
			ob.removeFrontLevel(oppositeSide)
		}
		if levelChanged {
			// After the level is removed, totalQty is 0; otherwise it's
			// whatever's left after the fills at this level.
			qty := ZeroQty
			if !levelEmpty {
				qty = level.totalQty
			}
			ob.emitDelta(oppositeSide, levelPrice, qty)
		}
	}

	return matches, remain
}

// applySTP handles a same-user crossing per the taker's STP mode. It
// mutates the book (and remain) directly and returns done=true when the
// taker should stop matching entirely.
func (ob *Orderbook) applySTP(mode STPMode, remain *Qty, node *OrderNode, level *MatchEngineEntry) (done bool) {
	resting := node.Order
	switch mode {
	case STPCancelTaker:
		// Halt matching; the resting order stays untouched and the
		// taker's remainder is dropped.
		*remain = ZeroQty
		return true

	case STPCancelResting:
		// Remove the resting order and continue with the next.
		level.removeNode(node)
		delete(ob.orderIndex, resting.ID)
		return false

	case STPCancelBoth:
		level.removeNode(node)
		delete(ob.orderIndex, resting.ID)
		*remain = ZeroQty
		return true

	case STPDecrement:
		// Net-out: both sides reduce by the smaller quantity. No trade
		// is reported (this is not a real match, just a cancellation
		// in disguise).
		fill := MinQty(*remain, resting.Volume)
		*remain = remain.Sub(fill)
		if fill.Eq(resting.Volume) {
			level.removeNode(node)
			delete(ob.orderIndex, resting.ID)
		} else {
			level.reduceHead(fill)
		}
		return false

	default:
		// Unknown STP mode -> treat as cancel-taker. Erring on the side
		// of "never let a same-user trade through" is the safer default
		// for a regulated venue.
		*remain = ZeroQty
		return true
	}
}

// frontLevel returns the best-priced level on the given side, or nil if
// that side is empty.
func (ob *Orderbook) frontLevel(side Side) *MatchEngineEntry {
	switch side {
	case BUY:
		if len(ob.Buy) == 0 {
			return nil
		}
		return ob.Buy[0]
	case SELL:
		if len(ob.Sell) == 0 {
			return nil
		}
		return ob.Sell[0]
	}
	return nil
}

func crosses(taker Order, restingPrice Px) bool {
	switch taker.Side {
	case BUY:
		return taker.Price.Gte(restingPrice)
	case SELL:
		return taker.Price.Lte(restingPrice)
	}
	return false
}

func makeMatch(taker, resting Order, price Px, volume Qty) Match {
	if taker.Side == BUY {
		return Match{BuyId: taker.ID, SellId: resting.ID, Price: price, Volume: volume}
	}
	return Match{BuyId: resting.ID, SellId: taker.ID, Price: price, Volume: volume}
}

// ---- proto round-trip ----

func OrderbookFromProto(p *protoModels.Orderbook) *Orderbook {
	ob := NewOrderbook(p.GetSymbol())
	ob.SetSeq(p.GetSeq())

	// rebuild bid ladder
	for _, entry := range p.GetBuy() {
		for _, protoOrder := range entry.GetOrders() {
			ob.Add(OrderFromProto(protoOrder))
		}
	}
	// rebuild ask ladder
	for _, entry := range p.GetSell() {
		for _, protoOrder := range entry.GetOrders() {
			ob.Add(OrderFromProto(protoOrder))
		}
	}
	for _, protoOrder := range p.ConditionalOrders {
		ob.ConditionalOrders = append(ob.ConditionalOrders, OrderFromProto(protoOrder))
	}
	return ob
}

func (ob *Orderbook) ToProto() *protoModels.Orderbook {
	out := &protoModels.Orderbook{Symbol: ob.Symbol, Seq: ob.seq}

	for _, level := range ob.Buy {
		entry := &protoModels.MatchEngineEntry{Price: level.Price.String()}
		for n := level.Orders.Head(); n != nil; n = n.Next {
			entry.Orders = append(entry.Orders, n.Order.ToProto())
		}
		out.Buy = append(out.Buy, entry)
	}
	for _, level := range ob.Sell {
		entry := &protoModels.MatchEngineEntry{Price: level.Price.String()}
		for n := level.Orders.Head(); n != nil; n = n.Next {
			entry.Orders = append(entry.Orders, n.Order.ToProto())
		}
		out.Sell = append(out.Sell, entry)
	}
	for _, order := range ob.ConditionalOrders {
		out.ConditionalOrders = append(out.ConditionalOrders, order.ToProto())
	}
	return out
}
