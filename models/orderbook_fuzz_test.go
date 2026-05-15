package models

// Native Go fuzzing for the order-book state machine. Whitebox (package
// models, not models_test) so we can inspect the bidByPrice / askByPrice
// maps and the orderIndex.
//
// Invariants verified after every op:
//
//  1. Buy is strictly descending by price; Sell is strictly ascending.
//  2. byPrice maps contain exactly one entry per level present in the
//     corresponding slice, pointing at the same struct.
//  3. orderIndex contains exactly one entry per resting order, with
//     the back-pointer to the correct level.
//  4. No level in either slice is empty.
//  5. Each level's totalQty equals the sum of its FIFO list's volumes.
//  6. No resting order has zero or negative volume.

import (
	"encoding/binary"
	"fmt"
	"testing"
)

const fuzzSymbol = "FUZZ_USDT"

// parseOps turns a byte slice into a sequence of structured ops. Each
// op is 22 bytes; trailing data shorter than that is ignored. We keep
// the encoding fixed-width so the corpus is portable.
type fuzzOp struct {
	kind     byte // 0=new, 1=cancel, 2=modify, rest=skip
	id       int64
	side     byte // 0=BUY, 1=SELL
	priceRaw int64
	volRaw   int64
	tif      byte
}

func parseOps(data []byte) []fuzzOp {
	const chunk = 22
	var out []fuzzOp
	for i := 0; i+chunk <= len(data); i += chunk {
		b := data[i : i+chunk]
		out = append(out, fuzzOp{
			kind:     b[0] % 4,
			id:       int64(binary.LittleEndian.Uint32(b[1:5]))%256 + 1, // bounded ID space for reuse
			side:     b[5] & 1,
			priceRaw: int64(binary.LittleEndian.Uint64(b[6:14]) % 1_000),  // 1..1000 whole units
			volRaw:   int64(binary.LittleEndian.Uint64(b[14:22]) % 100),   // 0..99 whole units
			tif:      0,                                                    // GTC; fuzz the matcher, not TIF
		})
	}
	return out
}

func opFromFuzz(o fuzzOp) Order {
	side := BUY
	if o.side == 1 {
		side = SELL
	}
	return Order{
		ID:     o.id,
		Symbol: fuzzSymbol,
		Side:   side,
		Type:   LIMIT,
		Price:  Px((o.priceRaw + 1) * 1_0000_0000), // ensure price >= 1
		Volume: Qty((o.volRaw + 1) * 1_0000_0000),  // ensure volume >= 1
	}
}

func FuzzOrderbookInvariants(f *testing.F) {
	// seeds: a few hand-crafted byte sequences that exercise crossing,
	// stacking, and cancel paths.
	f.Add([]byte{
		// new BUY id=1 price=10 vol=2
		0, 1, 0, 0, 0, 0, 10, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0,
		// new SELL id=2 price=10 vol=1 -> crosses
		0, 2, 0, 0, 0, 1, 10, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0,
		// cancel id=1
		1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	})
	f.Add([]byte{
		// many stacked buys at price 5
		0, 1, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0,
		0, 2, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0,
		0, 3, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0,
		// then a crossing sell that sweeps them
		0, 4, 0, 0, 0, 1, 5, 0, 0, 0, 0, 0, 0, 0, 99, 0, 0, 0, 0, 0, 0, 0,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		ob := NewOrderbook(fuzzSymbol)
		ops := parseOps(data)
		if len(ops) > 200 {
			// Cap to keep fuzz iterations fast; the property is local
			// to each op so longer sequences don't increase coverage.
			ops = ops[:200]
		}

		for i, op := range ops {
			applyFuzzOp(ob, op)
			if err := checkInvariants(ob); err != nil {
				t.Fatalf("invariant violated after op %d (%v): %v", i, op, err)
			}
		}
	})
}

func applyFuzzOp(ob *Orderbook, op fuzzOp) {
	switch op.kind {
	case 0: // new
		// Real exchange API layers reject submissions with an
		// already-resting orderID before the engine ever sees them;
		// the fuzz mirrors that contract so it tests realistic load
		// rather than producer bugs.
		if _, exists := ob.LookupOrder(op.id); exists {
			return
		}
		o := opFromFuzz(op)
		// Try to match first if we have crossing liquidity; rest the
		// remainder otherwise.
		_, remain := ob.MatchTaker(o, false)
		if remain.IsPositive() {
			o.Volume = remain
			ob.Add(o)
		}
	case 1: // cancel
		_ = ob.Cancel(op.id)
	case 2: // modify (reduce volume only — simplest case we can encode)
		o := opFromFuzz(op)
		if _, ok := ob.LookupOrder(op.id); ok {
			// reduce to half its current volume; if that's still positive
			_ = ob.ReduceVolume(op.id, o.Volume)
		}
	}
}

func checkInvariants(ob *Orderbook) error {
	// 1. sort order on each ladder
	for i := 1; i < len(ob.Buy); i++ {
		if !ob.Buy[i-1].Price.Gt(ob.Buy[i].Price) {
			return fmt.Errorf("Buy ladder not strictly descending at %d: %s >= %s",
				i, ob.Buy[i-1].Price.String(), ob.Buy[i].Price.String())
		}
	}
	for i := 1; i < len(ob.Sell); i++ {
		if !ob.Sell[i-1].Price.Lt(ob.Sell[i].Price) {
			return fmt.Errorf("Sell ladder not strictly ascending at %d: %s >= %s",
				i, ob.Sell[i-1].Price.String(), ob.Sell[i].Price.String())
		}
	}

	// 2. byPrice maps consistent with ladder slices
	if len(ob.bidByPrice) != len(ob.Buy) {
		return fmt.Errorf("bidByPrice has %d entries; Buy ladder has %d", len(ob.bidByPrice), len(ob.Buy))
	}
	for _, l := range ob.Buy {
		if got := ob.bidByPrice[l.Price]; got != l {
			return fmt.Errorf("bidByPrice[%s] does not point at slice entry", l.Price.String())
		}
	}
	if len(ob.askByPrice) != len(ob.Sell) {
		return fmt.Errorf("askByPrice has %d entries; Sell ladder has %d", len(ob.askByPrice), len(ob.Sell))
	}
	for _, l := range ob.Sell {
		if got := ob.askByPrice[l.Price]; got != l {
			return fmt.Errorf("askByPrice[%s] does not point at slice entry", l.Price.String())
		}
	}

	// 3. orderIndex consistent with what's actually resting
	seen := make(map[int64]bool)
	totalOrders := 0
	for _, ladder := range [][]*MatchEngineEntry{ob.Buy, ob.Sell} {
		for _, l := range ladder {
			for n := l.Orders.Head(); n != nil; n = n.Next {
				if seen[n.Order.ID] {
					return fmt.Errorf("orderID %d appears more than once across the book", n.Order.ID)
				}
				seen[n.Order.ID] = true
				totalOrders++

				if n.Level != l {
					return fmt.Errorf("orderID %d has stale Level back-pointer", n.Order.ID)
				}
				if got := ob.orderIndex[n.Order.ID]; got != n {
					return fmt.Errorf("orderIndex[%d] does not point at the node in the level's list", n.Order.ID)
				}
				if !n.Order.Volume.IsPositive() {
					return fmt.Errorf("orderID %d has non-positive volume %s",
						n.Order.ID, n.Order.Volume.String())
				}
			}
		}
	}
	if len(ob.orderIndex) != totalOrders {
		return fmt.Errorf("orderIndex has %d entries; %d resting orders observed",
			len(ob.orderIndex), totalOrders)
	}

	// 4. no empty levels in either slice
	for _, l := range ob.Buy {
		if l.Orders.IsEmpty() {
			return fmt.Errorf("empty Buy level at price %s left in ladder", l.Price.String())
		}
	}
	for _, l := range ob.Sell {
		if l.Orders.IsEmpty() {
			return fmt.Errorf("empty Sell level at price %s left in ladder", l.Price.String())
		}
	}

	// 5. totalQty equals the walking sum
	for _, ladder := range [][]*MatchEngineEntry{ob.Buy, ob.Sell} {
		for _, l := range ladder {
			var sum Qty
			for n := l.Orders.Head(); n != nil; n = n.Next {
				sum = sum.Add(n.Order.Volume)
			}
			if !sum.Eq(l.totalQty) {
				return fmt.Errorf("level @ %s: totalQty=%s, walking sum=%s",
					l.Price.String(), l.totalQty.String(), sum.String())
			}
		}
	}

	return nil
}
