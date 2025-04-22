package main

import (
	"fmt"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/joho/godotenv"

	"github.com/aliraad79/Gun/data"
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

	orderbooks := make(map[string]*data.Orderbook)

	log.Info("Starting Match Engine")

	for instrument := range instrumentChan {
		log.Debug("Processed:", instrument)

		switch instrument.Command {

		case NEW_ORDER_CMD:
			processNewOrder(orderbooks, instrument.Value)
		case CANCEL_ORDER_CMD:
			cancelOrder(orderbooks, instrument.Value)
		default:
			panic(fmt.Sprintf("unexpected main.Command: %#v", instrument.Command))
		}
	}

	wg.Wait()
}
