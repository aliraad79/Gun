// Package metrics exposes Gun's runtime telemetry as Prometheus
// collectors. The collectors are registered with the default registry
// at package init, so they show up on /metrics without any wiring at
// the caller's site.
//
// Conventions:
//
//   - All metric names start with "gun_".
//   - Order outcomes are tagged with a low-cardinality "result" label
//     (accepted / rejected_<reason>); rejection reasons are the same
//     stable matchEngine.Reject* constants the engine returns.
//   - Latency histograms use microsecond-scale buckets matching the
//     real distribution seen in Phase 2 latency measurement
//     (P50 ~344 ns, P99 ~5 µs, P99.9 ~50 µs).
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// engineLatencyBuckets covers ~250 ns up to 100 ms. Tightly packed in
// the 1–100 µs range where the engine actually lives; sparser at the
// extremes because anything past 1 ms is a tail event we want to count
// but don't need fine resolution on.
var engineLatencyBuckets = []float64{
	0.0000005, // 500 ns
	0.000001,  // 1 µs
	0.000002,
	0.000005,
	0.00001,
	0.00002,
	0.00005,
	0.0001,
	0.0002,
	0.0005,
	0.001,
	0.005,
	0.01,
	0.05,
	0.1,
}

var (
	ordersTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gun_orders_total",
			Help: "Count of orders the engine has seen, partitioned by symbol and outcome.",
		},
		[]string{"symbol", "result"},
	)

	matchesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gun_matches_total",
			Help: "Number of trade matches produced per symbol.",
		},
		[]string{"symbol"},
	)

	opDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gun_op_duration_seconds",
			Help:    "Wall-clock time spent in the engine per op, partitioned by symbol and op type.",
			Buckets: engineLatencyBuckets,
		},
		[]string{"symbol", "op"},
	)

	journalAppendDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gun_journal_append_duration_seconds",
			Help:    "Wall-clock time for a single journal append (including fsync if enabled).",
			Buckets: engineLatencyBuckets,
		},
	)

	bookLevels = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gun_book_levels",
			Help: "Current number of price levels per symbol per side.",
		},
		[]string{"symbol", "side"},
	)

	bookOrders = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gun_book_orders",
			Help: "Current number of resting orders per symbol per side.",
		},
		[]string{"symbol", "side"},
	)

	activeMarkets = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gun_active_markets",
			Help: "Number of Market goroutines currently running.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		ordersTotal,
		matchesTotal,
		opDuration,
		journalAppendDuration,
		bookLevels,
		bookOrders,
		activeMarkets,
	)
}

// Order records an order outcome for the given symbol. result is one of
// "accepted" or "rejected_<reason>".
func Order(symbol, result string) {
	ordersTotal.WithLabelValues(symbol, result).Inc()
}

// Matches records the number of matches produced by a single op.
func Matches(symbol string, n int) {
	if n <= 0 {
		return
	}
	matchesTotal.WithLabelValues(symbol).Add(float64(n))
}

// OpDuration records how long an op took. opName is "new", "cancel",
// or "modify".
func OpDuration(symbol, opName string, d time.Duration) {
	opDuration.WithLabelValues(symbol, opName).Observe(d.Seconds())
}

// JournalAppendDuration records how long one journal Append call took.
func JournalAppendDuration(d time.Duration) {
	journalAppendDuration.Observe(d.Seconds())
}

// BookDepth updates the depth gauges for a symbol. Call this from the
// Market goroutine after each op (or on a periodic timer) so dashboards
// see the resting-state evolve.
func BookDepth(symbol string, buyLevels, buyOrders, sellLevels, sellOrders int) {
	bookLevels.WithLabelValues(symbol, "buy").Set(float64(buyLevels))
	bookLevels.WithLabelValues(symbol, "sell").Set(float64(sellLevels))
	bookOrders.WithLabelValues(symbol, "buy").Set(float64(buyOrders))
	bookOrders.WithLabelValues(symbol, "sell").Set(float64(sellOrders))
}

// MarketCount sets the current count of active Market goroutines.
func MarketCount(n int) {
	activeMarkets.Set(float64(n))
}

// Handler returns the default Prometheus HTTP handler. Mount it on
// /metrics in your HTTP server.
func Handler() http.Handler {
	return promhttp.Handler()
}
