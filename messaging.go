package main

import (
	"github.com/aliraad79/Gun/models"
	log "github.com/sirupsen/logrus"
)

func publishResults(matches []models.Match) {
	log.Info("Publishing ", matches, " trade results to kafka or other mediums")
}

func publishOrderbook(orderbook *models.Orderbook) {
	if orderbook == nil {
		return
	}
	log.Info("Publishing ", orderbook, " orderbook to kafka or other mediums")
}
