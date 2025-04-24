package models

import (
	protoModels "github.com/aliraad79/Gun/models/models"
	"github.com/shopspring/decimal"
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
	STOP_lIMIT Type = "stop_limit"
)

type Order struct {
	ID           int64           `json:"id"`
	Symbol       string          `json:"symbol"`
	Side         Side            `json:"side"`
	Type         Type            `json:"type"`
	Price        decimal.Decimal `json:"price"`
	TriggerPrice decimal.Decimal `json:"trigger_price`
	Volume       decimal.Decimal `json:"volume"`
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
	return Order{
		ID:     order.GetId(),
		Symbol: order.GetSymbol(),
		Side:   Side(order.Side),
		Type:   Type(order.GetType()),
		Price:  decimal.RequireFromString(order.GetPrice()),
		Volume: decimal.RequireFromString(order.GetVolume()),
	}
}

func (order *Order) IsTriggered(price decimal.Decimal) bool {
	if order.Type != STOP_lIMIT {
		return true
	} else {
		switch order.Side {
		case BUY:
			return price.LessThanOrEqual(order.TriggerPrice)

		case SELL:
			return price.GreaterThanOrEqual(order.TriggerPrice)
		default:
			return false
		}
	}
}
