package main

import log "github.com/sirupsen/logrus"

func publishResults(matches []Match) {
	log.Info("Publishing ", matches, " trade results to kafka or other mediums")
}

func publishOrderbook(orderbook Orderbook) {
	log.Info("Publishing ", orderbook, " orderbook to kafka or other mediums")
}
