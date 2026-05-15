package main

import (
	"context"
	"net/http"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"github.com/aliraad79/Gun/journal"
	"github.com/aliraad79/Gun/market"
	"github.com/aliraad79/Gun/metrics"
	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	"github.com/aliraad79/Gun/tracing"
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

	// OpenTelemetry tracing. No-op unless OTEL_EXPORTER_OTLP_ENDPOINT is
	// set, so this is safe to call unconditionally.
	traceShutdown, err := tracing.Init(ctx)
	if err != nil {
		log.Error("tracing init: ", err)
	}
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = traceShutdown(shutCtx)
		shutCancel()
	}()

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

	// /metrics HTTP server. Listens on GUN_METRICS_ADDR (default :9090).
	// Empty string disables the endpoint (tests, sandboxed environments).
	metricsAddr := utils.GetEnvOrDefault("GUN_METRICS_ADDR", ":9090")
	var metricsSrv *http.Server
	if metricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		metricsSrv = &http.Server{
			Addr:         metricsAddr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		go func() {
			log.Warn("metrics endpoint listening on ", metricsAddr, "/metrics")
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Error("metrics server: ", err)
			}
		}()
	}

	instrumentChan := make(chan Instrument, 4096)
	wg.Add(1)
	go startConsumer(&wg, instrumentChan)

	log.Info("Starting Match Engine")

	var (
		startTimeMu sync.Mutex
		startTime   time.Time
	)

	// Periodic registry-size gauge: cheap to compute, gives dashboards a
	// "what's running" signal without polling the Registry directly.
	gaugeTick := time.NewTicker(2 * time.Second)
	defer gaugeTick.Stop()

	// Main dispatch loop. Exits when ctx is cancelled (SIGINT/SIGTERM)
	// — the inbox channel is then drained and each Market drains in
	// turn, joined via wg.
mainloop:
	for {
		select {
		case <-ctx.Done():
			log.Warn("shutdown signal received; draining markets")
			break mainloop
		case <-gaugeTick.C:
			metrics.MarketCount(registry.Count())
		case inst, ok := <-instrumentChan:
			if !ok {
				break mainloop
			}
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
	}

	// Graceful drain: cancel was already called by signal handler (or
	// happens via the deferred cancel above). Wait for all Market
	// goroutines to finish their inboxes, then stop the metrics server.
	cancel()
	wg.Wait()

	if metricsSrv != nil {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = metricsSrv.Shutdown(shutCtx)
		shutCancel()
	}
	log.Warn("Gun stopped cleanly")
}

func onMatch(symbol string, matches []models.Match) {
	go publishResults(matches)
}

func onBook(orderbook *models.Orderbook) {
	publishOrderbook(orderbook)
}
