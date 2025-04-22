package main

import (
	"github.com/aliraad79/Gun/data"
	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/persistance"
	log "github.com/sirupsen/logrus"
)

type Command string

const (
	NEW_ORDER_CMD    Command = "new_order"
	CANCEL_ORDER_CMD Command = "cancel_order"
)

type Instrument struct {
	Command Command
	Value   data.Order
}

func processNewOrder(orderbooks map[string]*data.Orderbook, order data.Order) {

	orderbook, err := matchEngine.LoadOrFetchOrderbook(orderbooks, order.Symbol)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}

	matches := matchEngine.AddNewOrder(orderbook, order)

	if len(matches) > 0 {
		matchEngine.HandleConditionalOrders(matches[0].Price)
		publishResults(matches)
	}
	publishOrderbook(*orderbook)

	persistance.CommitOrderBook(*orderbook)

}

func cancelOrder(orderbooks map[string]*data.Orderbook, order data.Order) {

	orderbook, err := matchEngine.LoadOrFetchOrderbook(orderbooks, order.Symbol)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}

	matchEngine.AddNewOrder(orderbook, order)

	publishOrderbook(*orderbook)

	persistance.CommitOrderBook(*orderbook)
}
