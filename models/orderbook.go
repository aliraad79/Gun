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
	Buy    []MatchEngineEntry
	Sell   []MatchEngineEntry
	Symbol string
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

func FromProto(protoOrderbook protoModels.Orderbook) *Orderbook {
	var Buys []MatchEngineEntry
	for _, entry := range protoOrderbook.GetBuy() {
		var orders []Order
		for _, order := range entry.GetOrders() {
			orders = append(orders, Order{
				ID:     order.GetId(),
				Symbol: order.GetSymbol(),
				Side:   Side(order.Side),
				Price:  decimal.RequireFromString(order.GetPrice()),
				Volume: decimal.RequireFromString(order.GetVolume()),
			})
		}
		Buys = append(Buys, MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var Sells []MatchEngineEntry
	for _, entry := range protoOrderbook.GetSell() {
		var orders []Order
		for _, order := range entry.GetOrders() {
			orders = append(orders, Order{
				ID:     order.GetId(),
				Symbol: order.GetSymbol(),
				Side:   Side(order.Side),
				Price:  decimal.RequireFromString(order.GetPrice()),
				Volume: decimal.RequireFromString(order.GetVolume()),
			})
		}
		Sells = append(Sells, MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	return &Orderbook{
		Buy:    Buys,
		Sell:   Sells,
		Symbol: protoOrderbook.GetSymbol(),
	}
}

func (orderbook *Orderbook) ToProto() *protoModels.Orderbook {
	var Buys []*protoModels.MatchEngineEntry
	for _, entry := range orderbook.Buy {
		var orders []*protoModels.Order
		for _, order := range entry.Orders {
			orders = append(orders, &protoModels.Order{
				Id:     order.ID,
				Symbol: order.Symbol,
				Side:   string(order.Side),
				Price:  order.Price.String(),
				Volume: order.Volume.String(),
			})
		}
		Buys = append(Buys, &protoModels.MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	var Sells []*protoModels.MatchEngineEntry
	for _, entry := range orderbook.Sell {
		var orders []*protoModels.Order
		for _, order := range entry.Orders {
			orders = append(orders, &protoModels.Order{
				Id:     order.ID,
				Symbol: order.Symbol,
				Side:   string(order.Side),
				Price:  order.Price.String(),
				Volume: order.Volume.String(),
			})
		}
		Sells = append(Sells, &protoModels.MatchEngineEntry{Orders: orders, Price: orders[0].Price})
	}
	return &protoModels.Orderbook{
		Buy:    Buys,
		Sell:   Sells,
		Symbol: orderbook.Symbol,
	}
}
