package main

import (
	"fmt"
	"math"
	"os"
	"strings"
)

type Order struct {
	ID     int64   `json:"id"`
	Symbol string  `json:"symbol"`
	Side   Side    `json:"side"`
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

type Side string

const (
	BUY  Side = "buy"
	SELL Side = "sell"
)

type Match struct {
	BuyId      int64   `json:"buy_id"`
	SellId     int64   `json:"sell_id"`
	MatchId    int64   `json:"match_id"`
	MatchPrice float64 `json:"match_price"`
}

type MatchEngineEntry struct {
	Price  float64
	Orders []Order
}

type Orderbook struct {
	buy  []MatchEngineEntry
	sell []MatchEngineEntry
}

func (orderbook *Orderbook) add(order Order) {
	switch order.Side {
	case BUY:
		{
			lastPirce := math.Inf(1)
			for idx, entry := range orderbook.buy {
				if entry.Price < order.Price && order.Price < lastPirce {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.buy = append(orderbook.buy[:idx], append([]MatchEngineEntry{newEntry}, orderbook.buy[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					entry.Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}
			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.buy = append(orderbook.buy, newEntry)

		}
	case SELL:
		{
			lastPirce := math.Inf(-1)
			for idx, entry := range orderbook.sell {
				if entry.Price > order.Price && order.Price > lastPirce {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.sell = append(orderbook.sell[:idx], append([]MatchEngineEntry{newEntry}, orderbook.sell[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					entry.Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}

			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.sell = append(orderbook.sell, newEntry)
		}
	default:
		panic(fmt.Sprintf("unexpected main.Side: %#v", order.Side))
	}
}

func createOrderbooks() map[string]Orderbook {
	supported_symbols := os.Getenv("SUPPORTED_SYMBOLS")
	orderbooks := make(map[string]Orderbook)
	for _, symbol := range strings.Split(supported_symbols, ",") {
		orderbooks[symbol] = Orderbook{buy: []MatchEngineEntry{}, sell: []MatchEngineEntry{}}
	}
	return orderbooks
}
