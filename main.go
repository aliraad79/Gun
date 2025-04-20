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
		matches := processOrders(orders)
		log.Println(matches)

		handleConditionalOrders(matches[0].match_price)
		publishResults(matches)
		publishOrderbook(orderbook)

	}

}
