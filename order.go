package main

type Order struct {
	id   int64
	side string
	price int64
}

type Match struct {
	buy_id   int64
	sell_id  int64
	match_id int64
	match_price int64
}

type Orderbook struct {
	buy  []Order
	sell []Order
}

func createOrder(id int64, side string) Order {
	return Order{id: id, side: side}
}

func createOrderbook() Orderbook {
	return Orderbook{buy: []Order{createOrder(1, "buy")}, sell: []Order{createOrder(2, "sell")}}
}

func createMatch() Match {
	sellOrder := Order{id: 1, side: "sell"}
	buyOrder := Order{id: 2, side: "buy"}
	return Match{buy_id: buyOrder.id, sell_id: sellOrder.id, match_id: 1}
}
