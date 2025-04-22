package main

import (
	log "github.com/sirupsen/logrus"
)

type Command string

const (
	NEW_ORDER_CMD Command = "new_order"
)

type Instrument struct {
	Command Command
	Value   Order
}

func processNewOrders(orderbooks map[string]*Orderbook, order Order) {

	orderbook, err := loadOrFetchOrderbook(orderbooks, order.Symbol)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}

	matches := processOrder(orderbook, order)

	if len(matches) > 0 {
		handleConditionalOrders(matches[0].Price)
		publishResults(matches)
	}
	publishOrderbook(*orderbook)

	commitOrderBook(*orderbook)

}
