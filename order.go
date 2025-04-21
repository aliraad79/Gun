package main

import "math"

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
	price  float64
	orders []Order
}

type Orderbook struct {
	buy  []MatchEngineEntry
	sell []MatchEngineEntry
}

func (orderbook Orderbook) add(order Order) {
	if order.Side == BUY {
		lastPirce := math.Inf(1)
		for _, entry := range orderbook.buy {
			if entry.price < order.Price && order.Price < lastPirce {
				entry.orders = append([]Order{order}, entry.orders...)
				break
			} else if entry.price == order.Price {
				entry.orders = append(entry.orders, order)
				break
			}
			lastPirce = entry.price

		}
	} else if order.Side == SELL {
		lastPirce := math.Inf(-1)
		for _, entry := range orderbook.sell {
			if entry.price > order.Price && order.Price > lastPirce {
				entry.orders = append([]Order{order}, entry.orders...)
				break
			} else if entry.price == order.Price {
				entry.orders = append(entry.orders, order)
				break
			}
			lastPirce = entry.price
		}
	}
}

func createOrder(id int64, side Side) Order {
	return Order{ID: id, Side: side}
}

func createOrderbook() Orderbook {
	return Orderbook{buy: []MatchEngineEntry{{price: 15.1, orders: []Order{createOrder(1, BUY)}}},
		sell: []MatchEngineEntry{{price: 15.1, orders: []Order{createOrder(1, BUY)}}}}
}
