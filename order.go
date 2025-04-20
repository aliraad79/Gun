package main

import "math"

type Order struct {
	id     int64
	side   Side
	price  float64
	volume float64
}

type Side int

const (
	BUY Side = iota
	SELL
)

type Match struct {
	buy_id      int64
	sell_id     int64
	match_id    int64
	match_price float64
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
	if order.side == BUY {
		lastPirce := math.Inf(1)
		for _, entry := range orderbook.buy {
			if entry.price < order.price && order.price < lastPirce {
				entry.orders = append([]Order{order}, entry.orders...)
				break
			} else if entry.price == order.price {
				entry.orders = append(entry.orders, order)
				break
			}
			lastPirce = entry.price

		}
	} else if order.side == SELL {
		lastPirce := math.Inf(-1)
		for _, entry := range orderbook.sell {
			if entry.price > order.price && order.price > lastPirce {
				entry.orders = append([]Order{order}, entry.orders...)
				break
			} else if entry.price == order.price {
				entry.orders = append(entry.orders, order)
				break
			}
			lastPirce = entry.price
		}
	}
}

func createOrder(id int64, side Side) Order {
	return Order{id: id, side: side}
}

func createOrderbook() Orderbook {
	return Orderbook{buy: []MatchEngineEntry{MatchEngineEntry{price: 15.1, orders: []Order{createOrder(1, BUY)}}},
		sell: []MatchEngineEntry{MatchEngineEntry{price: 15.1, orders: []Order{createOrder(1, BUY)}}}}
}

func createMatch() Match {
	sellOrder := Order{id: 1, side: SELL}
	buyOrder := Order{id: 2, side: BUY}
	return Match{buy_id: buyOrder.id, sell_id: sellOrder.id, match_id: 1}
}
