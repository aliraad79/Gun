package models

import (
	"github.com/shopspring/decimal"

	protoModels "github.com/aliraad79/Gun/models/models"
	log "github.com/sirupsen/logrus"
)

type MatchEngineEntry struct {
	Price  decimal.Decimal
	Orders []Order
}

type Orderbook struct {
	Buy               []MatchEngineEntry
	Sell              []MatchEngineEntry
	ConditionalOrders []Order
	Symbol            string
}

func (orderbook *Orderbook) Add(order Order) {
	switch order.Side {
	case BUY:
		{
			lastPirce := decimal.RequireFromString("100000000000000")
			for idx, entry := range orderbook.Buy {
				if entry.Price.LessThan(order.Price) && order.Price.LessThan(lastPirce) {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.Buy = append(orderbook.Buy[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Buy[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					orderbook.Buy[idx].Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}
			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.Buy = append(orderbook.Buy, newEntry)
		}
	case SELL:
		{
			lastPirce := decimal.Zero
			for idx, entry := range orderbook.Sell {
				if entry.Price.GreaterThan(order.Price) && order.Price.GreaterThan(lastPirce) {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.Sell = append(orderbook.Sell[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Sell[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					orderbook.Sell[idx].Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}

			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.Sell = append(orderbook.Sell, newEntry)
		}
	default:
		log.Error("unexpected main.Side: ", order.Side)
	}
}

func OrderbookFromProto(protoOrderbook protoModels.Orderbook) *Orderbook {
	var Buys []MatchEngineEntry
	for _, entry := range protoOrderbook.GetBuy() {
		var orders []Order
		for _, order := range entry.GetOrders() {
			orders = append(orders, OrderFromProto(order))
		}
		Buys = append(Buys, MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var Sells []MatchEngineEntry
	for _, entry := range protoOrderbook.GetSell() {
		var orders []Order
		for _, order := range entry.GetOrders() {
			orders = append(orders, OrderFromProto(order))
		}
		Sells = append(Sells, MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var conditionalOrders []Order
	for _, order := range protoOrderbook.ConditionalOrders {
		conditionalOrders = append(conditionalOrders, OrderFromProto(order))
	}
	return &Orderbook{
		Buy:               Buys,
		Sell:              Sells,
		Symbol:            protoOrderbook.GetSymbol(),
		ConditionalOrders: conditionalOrders,
	}
}

func (orderbook *Orderbook) ToProto() *protoModels.Orderbook {
	var Buys []*protoModels.MatchEngineEntry
	for _, entry := range orderbook.Buy {
		var orders []*protoModels.Order
		for _, order := range entry.Orders {
			orders = append(orders, order.ToProto())
		}
		Buys = append(Buys, &protoModels.MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var Sells []*protoModels.MatchEngineEntry
	for _, entry := range orderbook.Sell {
		var orders []*protoModels.Order
		for _, order := range entry.Orders {
			orders = append(orders, order.ToProto())
		}
		Sells = append(Sells, &protoModels.MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var conditionalOrders []*protoModels.Order
	for _, order := range orderbook.ConditionalOrders {
		conditionalOrders = append(conditionalOrders, order.ToProto())
	}
	return &protoModels.Orderbook{
		Buy:               Buys,
		Sell:              Sells,
		Symbol:            orderbook.Symbol,
		ConditionalOrders: conditionalOrders,
	}
}

func (Orderbook *Orderbook) AddConditionalOrder(order Order) {
	Orderbook.ConditionalOrders = append(Orderbook.ConditionalOrders, order)
}
