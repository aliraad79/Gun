package main

import (
	"os"
	"strings"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

type Order struct {
	ID     int64           `json:"id"`
	Symbol string          `json:"symbol"`
	Side   Side            `json:"side"`
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

type Side string

const (
	BUY  Side = "buy"
	SELL Side = "sell"
)

type Match struct {
	BuyId      int64           `json:"buy_id"`
	SellId     int64           `json:"sell_id"`
	MatchId    int64           `json:"match_id"`
	MatchPrice decimal.Decimal `json:"match_price"`
}

type MatchEngineEntry struct {
	Price  decimal.Decimal
	Orders []Order
}

type Orderbook struct {
	Buy  []MatchEngineEntry
	Sell []MatchEngineEntry
}

func (orderbook *Orderbook) add(order Order) {
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

func createOrderbooks() map[string]*Orderbook {
	supported_symbols := os.Getenv("SUPPORTED_SYMBOLS")
	orderbooks := make(map[string]*Orderbook)
	for _, symbol := range strings.Split(supported_symbols, ",") {
		orderbooks[symbol] = &Orderbook{}
	}
	return orderbooks
}
