package main

import "log"

func publishResults(matches []Match) {
	log.Println("Publishing", matches, " trade results to kafka or other mediums")
}

func publishOrderbook(orderbook Orderbook) {
	log.Println("Publishing", orderbook, "orderbook to kafka or other mediums")
}
