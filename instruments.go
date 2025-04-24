package main

import (
	"sync"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	"github.com/aliraad79/Gun/utils"
	"github.com/go-redis/redis"
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

func processNewOrder(orderbooks map[string]*models.Orderbook, mutexes map[string]*sync.Mutex, rdb *redis.Client, order models.Order) {

	orderbook, err := matchEngine.LoadOrFetchOrderbook(orderbooks, order.Symbol, rdb)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}

	mutex := utils.GetOrCreateMutex(mutexes, order.Symbol)
	mutex.Lock()

	matches := matchEngine.AddNewOrder(orderbook, order)

	if len(matches) > 0 {
		matchEngine.HandleConditionalOrders(matches[0].Price)
		publishResults(matches)
	}
	publishOrderbook(*orderbook)

	persistance.CommitOrderBook(*orderbook, rdb)
	mutex.Unlock()
}

func cancelOrder(orderbooks map[string]*models.Orderbook, mutexes map[string]*sync.Mutex, rdb *redis.Client, order models.Order) {

	orderbook, err := matchEngine.LoadOrFetchOrderbook(orderbooks, order.Symbol, rdb)
	if err != nil {
		log.Error("No orderbook was found for ", order.Symbol)
		return
	}
	mutex := utils.GetOrCreateMutex(mutexes, order.Symbol)
	mutex.Lock()

	matchEngine.CancelOrder(orderbook, order)

	publishOrderbook(*orderbook)

	persistance.CommitOrderBook(*orderbook, rdb)
	mutex.Unlock()
}
