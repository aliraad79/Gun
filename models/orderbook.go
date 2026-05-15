package models

import (
	protoModels "github.com/aliraad79/Gun/models/models"
	log "github.com/sirupsen/logrus"
)

type MatchEngineEntry struct {
	Price  Px
	Orders []Order
}

type Orderbook struct {
	Buy               []MatchEngineEntry
	Sell              []MatchEngineEntry
	ConditionalOrders []Order
	Symbol            string
}

// Add places an order on its side of the book at the correct price level.
// Buy levels are stored in descending price order (best bid first); sell
// levels are stored in ascending price order (best ask first). Orders at
// the same level are appended in arrival order (FIFO head -> tail).
//
// Today this is an O(n) linear walk; Phase 2d replaces it with a
// binary-searched insert plus an O(1) level-by-price map.
func (orderbook *Orderbook) Add(order Order) {
	switch order.Side {
	case BUY:
		for idx, entry := range orderbook.Buy {
			if order.Price.Gt(entry.Price) {
				newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
				orderbook.Buy = append(orderbook.Buy[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Buy[idx:]...)...)
				return
			}
			if order.Price.Eq(entry.Price) {
				orderbook.Buy[idx].Orders = append(entry.Orders, order)
				return
			}
		}
		orderbook.Buy = append(orderbook.Buy, MatchEngineEntry{Orders: []Order{order}, Price: order.Price})

	case SELL:
		for idx, entry := range orderbook.Sell {
			if order.Price.Lt(entry.Price) {
				newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
				orderbook.Sell = append(orderbook.Sell[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Sell[idx:]...)...)
				return
			}
			if order.Price.Eq(entry.Price) {
				orderbook.Sell[idx].Orders = append(entry.Orders, order)
				return
			}
		}
		orderbook.Sell = append(orderbook.Sell, MatchEngineEntry{Orders: []Order{order}, Price: order.Price})

	default:
		log.Error("unexpected order.Side: ", order.Side)
	}
}

func OrderbookFromProto(protoOrderbook *protoModels.Orderbook) *Orderbook {
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
