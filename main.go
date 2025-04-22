package main

import (
	"fmt"
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
	instrumentChan := make(chan Instrument, 1000)
	wg.Add(1)
	go startConsumer(&wg, instrumentChan)

	orderbooks := make(map[string]*Orderbook)

	log.Info("Starting Match Engine")

	for instrument := range instrumentChan {
		log.Debug("Processed:", instrument)

		switch instrument.Command {

		case NEW_ORDER_CMD:
			processNewOrders(orderbooks, instrument.Value)
		default:
			panic(fmt.Sprintf("unexpected main.Command: %#v", instrument.Command))
		}
	}

	wg.Wait()
}
