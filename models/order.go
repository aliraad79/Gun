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

type Order struct {
	ID           int64  `json:"id"`
	Symbol       string `json:"symbol"`
	Side         Side   `json:"side"`
	Type         Type   `json:"type"`
	Price        Px     `json:"price"`
	TriggerPrice Px     `json:"trigger_price"`
	Volume       Qty    `json:"volume"`
}

func (order *Order) ToProto() *protoModels.Order {
	return &protoModels.Order{
		Id:     order.ID,
		Symbol: order.Symbol,
		Side:   string(order.Side),
		Price:  order.Price.String(),
		Volume: order.Volume.String(),
		Type:   string(order.Type),
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
	return Order{
		ID:     order.GetId(),
		Symbol: order.GetSymbol(),
		Side:   Side(order.Side),
		Type:   Type(order.GetType()),
		Price:  price,
		Volume: vol,
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
