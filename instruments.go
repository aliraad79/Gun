package main

import (
	"sync"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	log "github.com/sirupsen/logrus"
)

type Command string

const (
	NEW_ORDER_CMD      Command = "new_order"
	CANCEL_ORDER_CMD   Command = "cancel_order"
	START_LOADTEST_CMD Command = "start_loadtest"
	END_LOADTEST_CMD   Command = "end_loadtest"
)

type Instrument struct {
	Command Command
	Value   models.Order
}

func processNewOrder(mutex *sync.Mutex, order models.Order) {
	if err := models.Validate(order); err != nil {
		log.Warn("Invalid Order is detected. ", order)
		return
	}
	mutex.Lock()

	orderbook, err := matchEngine.LoadOrFetchOrderbook(order.Symbol)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}

	matches := matchEngine.MatchAndAddNewOrder(orderbook, order)

	if len(matches) > 0 {
		newMatches := matchEngine.HandleConditionalOrders(orderbook, matches)
		matches = append(matches, newMatches...)
		go publishResults(matches)
	}

	persistance.CommitOrderBook(*orderbook)
	mutex.Unlock()

	publishOrderbook(*orderbook)
}

func cancelOrder(mutex *sync.Mutex, order models.Order) {
	mutex.Lock()

	orderbook, err := matchEngine.LoadOrFetchOrderbook(order.Symbol)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}
	matchEngine.CancelOrder(orderbook, order)

	persistance.CommitOrderBook(*orderbook)
	mutex.Unlock()

	publishOrderbook(*orderbook)
}
