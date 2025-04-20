package main

import "log"

func handleConditionalOrders(lastMatchPrice int64) {
	log.Println("handeling conditional orders based on", lastMatchPrice)
}
func processOrders(orders []Order) []Match {
	return []Match{createMatch()}
}
