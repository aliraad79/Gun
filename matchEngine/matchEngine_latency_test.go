//go:build !race

// Latency percentile measurement.
//
// Skipped under `go test -race` (the race detector adds substantial,
// non-uniform overhead that makes percentile numbers meaningless). Also
// skipped under `go test -short`.
//
// Run with:
//
//   go test -run=TestLatencyPercentiles -v ./matchEngine/...
//
// Output is appended to bench/phase-2-latency.txt and logged via t.Logf.

package matchEngine_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
)

func TestLatencyPercentiles_EndToEndMixed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency measurement under -short")
	}

	const (
		warmupOps   = 5_000
		measuredOps = 200_000
	)

	mid := int64(10_000)
	ob := buildBook(2000, 200, mid)
	r := rand.New(rand.NewPCG(7, 42))

	var nextID int64 = 10_000_000

	// Warm up so the JIT / allocator / GC reach steady state before we
	// start recording. Without this, the first few thousand samples are
	// systematically slower and skew P99.
	for i := 0; i < warmupOps; i++ {
		runMixedOp(ob, r, &nextID)
	}

	// Force GC right before measurement so a stop-the-world during the
	// recording window doesn't corrupt P99.9 / P99.99.
	runtime.GC()

	samples := make([]int64, measuredOps)
	for i := 0; i < measuredOps; i++ {
		start := time.Now()
		runMixedOp(ob, r, &nextID)
		samples[i] = time.Since(start).Nanoseconds()
	}

	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	pct := func(p float64) int64 {
		idx := int(float64(len(samples)-1) * p / 100.0)
		return samples[idx]
	}

	p50 := pct(50)
	p90 := pct(90)
	p99 := pct(99)
	p999 := pct(99.9)
	p9999 := pct(99.99)
	pmax := samples[len(samples)-1]

	// Roughly: total ops / measured-window seconds. Useful as a sanity
	// check that the benchmark and the latency test agree on order of
	// magnitude, but not directly comparable to BenchmarkEndToEndMixed
	// (which has its own overhead from b.ResetTimer / b.N machinery).
	var total int64
	for _, s := range samples {
		total += s
	}
	avgNs := total / int64(len(samples))

	report := fmt.Sprintf(
		"end-to-end mixed (70%% post / 20%% cross / 10%% cancel, depth ~2000)\n"+
			"  samples : %d (after %d warmup ops)\n"+
			"  avg     : %s\n"+
			"  p50     : %s\n"+
			"  p90     : %s\n"+
			"  p99     : %s\n"+
			"  p99.9   : %s\n"+
			"  p99.99  : %s\n"+
			"  max     : %s\n",
		len(samples), warmupOps,
		time.Duration(avgNs),
		time.Duration(p50),
		time.Duration(p90),
		time.Duration(p99),
		time.Duration(p999),
		time.Duration(p9999),
		time.Duration(pmax),
	)

	t.Log("\n" + report)

	if path := os.Getenv("GUN_LATENCY_OUT"); path != "" {
		writeReport(t, path, report)
	} else if wd, err := os.Getwd(); err == nil {
		// matchEngine/ is one level below repo root; bench/ is the sibling.
		out := filepath.Join(wd, "..", "bench", "phase-2-latency.txt")
		writeReport(t, out, report)
	}
}

func writeReport(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Logf("could not create %s: %v", filepath.Dir(path), err)
		return
	}
	header := fmt.Sprintf("# Gun latency percentiles\n# Generated: %s\n# Host: %s\n\n",
		time.Now().UTC().Format(time.RFC3339), runtime.Version())
	if err := os.WriteFile(path, []byte(header+body), 0o644); err != nil {
		t.Logf("could not write %s: %v", path, err)
	}
}

func runMixedOp(ob *models.Orderbook, r *rand.Rand, nextID *int64) {
	*nextID++
	roll := r.Float64()
	side := models.BUY
	if r.Float64() < 0.5 {
		side = models.SELL
	}
	mid := int64(10_000)

	switch {
	case roll < 0.70: // post liquidity away from best
		offset := int64(2 + r.IntN(50))
		price := p(mid - offset)
		if side == models.SELL {
			price = p(mid + offset)
		}
		vol := models.Qty(5000_0000 + r.Int64N(1_0000_0000))
		_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
			ID: *nextID, Symbol: "BTC_USDT", Type: models.LIMIT, Side: side,
			Price: price, Volume: vol,
		})
	case roll < 0.90: // taker that crosses
		price := p(mid + 5)
		if side == models.SELL {
			price = p(mid - 5)
		}
		vol := models.Qty(5000_0000 + r.Int64N(1_0000_0000))
		_ = matchEngine.MatchAndAddNewOrder(ob, models.Order{
			ID: *nextID, Symbol: "BTC_USDT", Type: models.LIMIT, Side: side,
			Price: price, Volume: vol,
		})
	default: // cancel best-priced resting order if any exist
		book := ob.Buy
		if side == models.SELL {
			book = ob.Sell
		}
		if len(book) > 0 && book[0].Orders.Head() != nil {
			_ = matchEngine.CancelOrder(ob, book[0].Orders.Head().Order.ID)
		}
	}
}
