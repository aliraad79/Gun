package main

import (
	"log"
)

func main() {
	log.Println("Starting Match Engine")

	orderbook := createOrderbook()

	for {
		orders := receiveOrders()
		log.Println(orders)

		for _, order := range orders {
			matches := processOrders(orderbook, order)

			if len(matches) > 0 {
				handleConditionalOrders(matches[0].match_price)
				publishResults(matches)
			}
			publishOrderbook(orderbook)
		}
	}

}
