package models

// Match is one fill produced by the matching engine. Seq is a per-symbol
// strictly monotonic sequence number assigned by Orderbook when the
// match is recorded — consumers can detect gaps in their replay stream
// by tracking the last seq they observed per symbol.
type Match struct {
	Seq     uint64 `json:"seq"`
	BuyId   int64  `json:"buy_id"`
	SellId  int64  `json:"sell_id"`
	MatchId int64  `json:"match_id"`
	Price   Px     `json:"price"`
	Volume  Qty    `json:"volume"`
}
