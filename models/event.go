package models

// BookDelta is a Level 2 (aggregated-by-price) order-book update. It is
// emitted whenever the total resting quantity at a price level changes
// — order added, order cancelled, order partially or fully matched, or
// in-place quantity reduction.
//
// Seq is drawn from the Orderbook's monotonic counter, so consumers can
// detect gaps in their feed and request a re-snapshot. A Qty of zero
// means "this level no longer exists" (it was fully consumed or all
// orders at it cancelled).
type BookDelta struct {
	Seq    uint64 `json:"seq"`
	Symbol string `json:"symbol"`
	Side   Side   `json:"side"`
	Price  Px     `json:"price"`
	Qty    Qty    `json:"qty"`
}

// L2Sink is the callback signature for L2 fan-out. It is invoked
// synchronously by the Orderbook on the symbol's processing goroutine;
// if the consumer needs async handling it must spawn its own goroutine
// inside the callback.
type L2Sink func(BookDelta)
