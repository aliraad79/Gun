package data

import (
	"github.com/shopspring/decimal"

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
