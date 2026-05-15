package models

import (
	"errors"

	protoModels "github.com/aliraad79/Gun/models/models"
)

type Side string

const (
	BUY  Side = "buy"
	SELL Side = "sell"
)

type Type string

const (
	LIMIT      Type = "limit"
	MARKET     Type = "market"
	STOP_LIMIT Type = "stop_limit"
)

// TIF (Time-in-Force) controls what happens to an order's unfilled
// remainder after a single matching pass.
type TIF string

const (
	// GTC (good-til-cancelled) is the default. Unfilled remainder rests
	// on the book until matched or cancelled.
	GTC TIF = ""

	// IOC (immediate-or-cancel) matches what crosses immediately and
	// drops the unfilled remainder. Useful for taker-only strategies.
	IOC TIF = "ioc"

	// FOK (fill-or-kill) requires that the order can be fully matched
	// in a single pass; otherwise the entire order is rejected without
	// any partial fill.
	FOK TIF = "fok"

	// PostOnly rejects the order if it would cross the spread (i.e.
	// take liquidity). Used by market makers to guarantee a maker fee
	// tier on every order.
	PostOnly TIF = "post_only"
)

// STPMode (Self-Trade Prevention) controls what happens when a taker
// crosses against a resting order belonging to the same UserID.
type STPMode string

const (
	// STPNone disables self-trade prevention. Same-user trades will
	// match like any other. Default when UserID == 0 (legacy orders).
	STPNone STPMode = ""

	// STPCancelTaker (the safe default when UserID is set) cancels the
	// remainder of the incoming taker against any of its own orders.
	STPCancelTaker STPMode = "cancel_taker"

	// STPCancelResting cancels the resting order so the taker can
	// proceed and either match the next-best resting or rest itself.
	STPCancelResting STPMode = "cancel_resting"

	// STPCancelBoth cancels both sides.
	STPCancelBoth STPMode = "cancel_both"

	// STPDecrement reduces both sides by the smaller of the two
	// quantities and cancels the resting; the taker may then continue
	// against other resting orders.
	STPDecrement STPMode = "decrement"
)

type Order struct {
	ID           int64   `json:"id"`
	Symbol       string  `json:"symbol"`
	UserID       int64   `json:"user_id,omitempty"`
	Side         Side    `json:"side"`
	Type         Type    `json:"type"`
	TimeInForce  TIF     `json:"time_in_force,omitempty"`
	STP          STPMode `json:"stp,omitempty"`
	Price        Px      `json:"price"`
	TriggerPrice Px      `json:"trigger_price"`
	Volume       Qty     `json:"volume"`
}

// Normalize fills in defaults for optional fields. Safe to call repeatedly.
// New code should call this before submitting an order; the matching
// engine also calls it defensively.
func (order *Order) Normalize() {
	if order.TimeInForce == "" {
		order.TimeInForce = GTC
	}
	if order.STP == "" && order.UserID != 0 {
		// If the caller set UserID but not STP, default to the safe
		// behavior: cancel the taker's unfilled remainder against
		// itself. Anything else would let the same-user trade through.
		order.STP = STPCancelTaker
	}
}

func (order *Order) ToProto() *protoModels.Order {
	triggerStr := ""
	if !order.TriggerPrice.IsZero() {
		triggerStr = order.TriggerPrice.String()
	}
	return &protoModels.Order{
		Id:           order.ID,
		Symbol:       order.Symbol,
		Side:         string(order.Side),
		Price:        order.Price.String(),
		Volume:       order.Volume.String(),
		Type:         string(order.Type),
		TriggerPrice: triggerStr,
		UserId:       order.UserID,
		TimeInForce:  string(order.TimeInForce),
		Stp:          string(order.STP),
	}
}

func OrderFromProto(order *protoModels.Order) Order {
	// Persisted orderbook snapshots are produced by this engine, so the
	// strings are guaranteed to be in canonical fixed-point form. A parse
	// error here means the snapshot is corrupt — panic so we don't quietly
	// re-load a bad book.
	price, err := ParsePx(order.GetPrice())
	if err != nil {
		panic("models: corrupt order price in snapshot: " + err.Error())
	}
	vol, err := ParseQty(order.GetVolume())
	if err != nil {
		panic("models: corrupt order volume in snapshot: " + err.Error())
	}
	var trigger Px
	if s := order.GetTriggerPrice(); s != "" {
		trigger, err = ParsePx(s)
		if err != nil {
			panic("models: corrupt trigger price in snapshot: " + err.Error())
		}
	}
	return Order{
		ID:           order.GetId(),
		Symbol:       order.GetSymbol(),
		UserID:       order.GetUserId(),
		Side:         Side(order.Side),
		Type:         Type(order.GetType()),
		TimeInForce:  TIF(order.GetTimeInForce()),
		STP:          STPMode(order.GetStp()),
		Price:        price,
		TriggerPrice: trigger,
		Volume:       vol,
	}
}

// IsTriggered reports whether a stop-limit order should be promoted to a
// live limit order at the given reference price. Non-stop orders are always
// considered triggered.
func (order *Order) IsTriggered(price Px) bool {
	if order.Type != STOP_LIMIT {
		return true
	}
	switch order.Side {
	case BUY:
		return price.Lte(order.TriggerPrice)
	case SELL:
		return price.Gte(order.TriggerPrice)
	default:
		return false
	}
}

// ErrNotValidOrder is returned by Validate for malformed orders.
var ErrNotValidOrder = errors.New("invalid order")

func Validate(order Order) error {
	if order.Type == "" || order.Side == "" || order.Price.IsZero() || order.Symbol == "" {
		return ErrNotValidOrder
	}
	return nil
}
