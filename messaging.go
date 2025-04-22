package main

import (
	"github.com/aliraad79/Gun/data"
	log "github.com/sirupsen/logrus"
)

func publishResults(matches []data.Match) {
	log.Info("Publishing ", matches, " trade results to kafka or other mediums")
}

func publishOrderbook(orderbook data.Orderbook) {
	log.Info("Publishing ", orderbook, " orderbook to kafka or other mediums")
}
