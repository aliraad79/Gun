package main

import (
	"context"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/market"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	"github.com/aliraad79/Gun/utils"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Warn("Error loading .env file")
	}

	log.SetLevel(log.WarnLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	persistance.InitClient()

	// Durability is mandatory. Configure via env:
	//   GUN_JOURNAL_DIR     - directory for per-symbol journals (default ./data/journal)
	//   GUN_JOURNAL_FSYNC   - "true" to fsync every append (default "true")
	journalDir := utils.GetEnvOrDefault("GUN_JOURNAL_DIR", "./data/journal")
	fsyncOnAppend := true
	if v := utils.GetEnvOrDefault("GUN_JOURNAL_FSYNC", "true"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			fsyncOnAppend = b
		}
	}
	j, err := journal.NewFileJournal(journalDir, fsyncOnAppend)
	if err != nil {
		log.Fatal("could not open journal at ", journalDir, ": ", err)
	}
	defer j.Close()
	log.Warn("journal active at ", journalDir, " (fsync=", fsyncOnAppend, ")")

	var wg sync.WaitGroup

	registry := market.NewRegistry(ctx, &wg, market.Options{
		InboxSize: 4096,
		OnMatch:   onMatch,
		OnBook:    onBook,
		Journal:   j,
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
