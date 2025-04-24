package main

import (
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/joho/godotenv"

	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
)

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	log.SetLevel(log.WarnLevel)

	var wg sync.WaitGroup
	instrumentChan := make(chan Instrument, 1000)
	wg.Add(1)
	go startConsumer(&wg, instrumentChan)

	rdb := persistance.RedisClient()

	orderbooks := make(map[string]*models.Orderbook)
	mutexes := make(map[string]*sync.Mutex)

	log.Info("Starting Match Engine")

	var mu sync.Mutex
	startTime := time.Now()
	for i := 0; i < 10; i++ {
		go func() {
			for instrument := range instrumentChan {
				log.Debug("Processed:", instrument)

				switch instrument.Command {
				case NEW_ORDER_CMD:
					processNewOrder(orderbooks, mutexes, rdb, instrument.Value)
				case CANCEL_ORDER_CMD:
					cancelOrder(orderbooks, mutexes, rdb, instrument.Value)
				case END_LOADTEST_CMD:
					mu.Lock()
					log.Warn("Load test ended in ", time.Since(startTime))
					mu.Unlock()
				case START_LOADTEST_CMD:
					mu.Lock()
					startTime = time.Now()
					log.Warn("Load test started in ", startTime)
					mu.Unlock()
				default:
					panic(fmt.Sprintf("unexpected main.Command: %#v", instrument.Command))
				}
			}
		}()
	}
	wg.Wait()
}
