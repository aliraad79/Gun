package main

import (
	"context"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/aliraad79/Gun/market"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Warn("Error loading .env file")
	}

	log.SetLevel(log.WarnLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	persistance.InitClient()

	var wg sync.WaitGroup

	registry := market.NewRegistry(ctx, &wg, market.Options{
		InboxSize: 4096,
		OnMatch:   onMatch,
		OnBook:    onBook,
		Persist:   true,
	})

	instrumentChan := make(chan Instrument, 4096)
	wg.Add(1)
	go startConsumer(&wg, instrumentChan)

	log.Info("Starting Match Engine")

	var (
		startTimeMu sync.Mutex
		startTime   time.Time
	)

	for inst := range instrumentChan {
		switch inst.Command {
		case NEW_ORDER_CMD:
			registry.Submit(inst.Value)
		case CANCEL_ORDER_CMD:
			registry.Cancel(inst.Value)
		case START_LOADTEST_CMD:
			startTimeMu.Lock()
			startTime = time.Now()
			log.Warn("Load test started at ", startTime)
			startTimeMu.Unlock()
		case END_LOADTEST_CMD:
			startTimeMu.Lock()
			log.Warn("Load test ended in ", time.Since(startTime),
				" across ", registry.Count(), " markets")
			startTimeMu.Unlock()
		}
	}

	cancel()
	wg.Wait()
}

func onMatch(symbol string, matches []models.Match) {
	go publishResults(matches)
}

func onBook(orderbook *models.Orderbook) {
	publishOrderbook(orderbook)
}
