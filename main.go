package main

import (
	"log"
	"sync"
)

func main() {
	var wg sync.WaitGroup
	msgChan := make(chan Order)
	wg.Add(1)
	go startConsumer(&wg, msgChan)

	log.Println("Starting Match Engine")

	orderbook := createOrderbook()

	for order := range msgChan {
		log.Println("Processed:", order)

		matches := processOrders(orderbook, order)

		if len(matches) > 0 {
			handleConditionalOrders(matches[0].match_price)
			publishResults(matches)
		}
		publishOrderbook(orderbook)
	}
	wg.Wait()
}
