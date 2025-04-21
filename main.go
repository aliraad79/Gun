package main

import (
	"log"
	"sync"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	var wg sync.WaitGroup
	msgChan := make(chan Order)
	wg.Add(1)
	go startConsumer(&wg, msgChan)

	log.Println("Starting Match Engine")

	orderbooks := createOrderbooks()

	for order := range msgChan {
		log.Println("Processed:", order)
		orderbook := orderbooks[order.Symbol]

		matches := processOrders(orderbook, order)

		if len(matches) > 0 {
			handleConditionalOrders(matches[0].MatchPrice)
			publishResults(matches)
		}
		publishOrderbook(orderbook)
	}
	wg.Wait()
}
