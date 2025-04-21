package main

import (
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.SetLevel(log.DebugLevel)

	var wg sync.WaitGroup
	msgChan := make(chan Order)
	wg.Add(1)
	go startConsumer(&wg, msgChan)

	log.Info("Starting Match Engine")

	orderbooks := createOrderbooks()

	for order := range msgChan {
		log.Debug("Processed:", order)
		orderbook := orderbooks[order.Symbol]

		matches := processOrder(orderbook, order)

		log.Warn(orderbook)

		if len(matches) > 0 {
			handleConditionalOrders(matches[0].MatchPrice)
			publishResults(matches)
		}
		publishOrderbook(*orderbook)
	}
	wg.Wait()
}
