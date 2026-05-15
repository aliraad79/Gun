# Gun

> A modern, in-memory matching engine for spot markets, written in Go.

[![CI](https://github.com/aliraad79/Gun/actions/workflows/ci.yml/badge.svg)](https://github.com/aliraad79/Gun/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/aliraad79/Gun.svg)](https://pkg.go.dev/github.com/aliraad79/Gun)

Gun is a continuous-trading limit order book matching engine. It accepts new
orders from Kafka, matches them against an in-memory order book with strict
price–time priority, and persists snapshots to Redis. Every active symbol
runs on its own goroutine with no shared mutable state, so throughput
scales linearly with active markets on a multi-core box.

Gun is designed to be embedded as the matching core of a larger exchange
stack — alongside an API gateway, risk/credit checks, market-data fan-out,
and clearing. It owns *one* concern: matching, fast and correctly.

---

## Features

| Feature | Status |
|---|---|
| Limit, market, and stop-limit orders | ✅ |
| Price–time priority (FIFO at each level) | ✅ |
| Per-symbol single-writer execution (lock-free) | ✅ |
| O(1) cancel via orderID index | ✅ |
| O(log n) insert via binary-searched price ladder | ✅ |
| Fixed-point arithmetic (8-decimal scaled int64) | ✅ |
| Snapshot persistence to Redis (protobuf) | ✅ |
| Conditional / triggered order re-evaluation | ✅ |
| Kafka-driven order ingest | ✅ |
| Self-trade prevention (STP) | 🛠 planned (Phase 3) |
| IOC / FOK / post-only time-in-force | 🛠 planned (Phase 3) |
| Order modify / replace | 🛠 planned (Phase 3) |
| L2 market-data publishing | 🛠 planned (Phase 3) |
| Sequence numbers + gap detection | 🛠 planned (Phase 3) |
| Journal-based recovery | 🛠 planned (Phase 3) |

---

## Architecture

```
                ┌──────────┐        ┌─────────────────────────────────────┐
   orders ────► │  Kafka   │ ─────► │            Gun (this repo)          │
                └──────────┘        │                                     │
                                    │   ┌─────────────────────────────┐   │
                                    │   │  Registry (lazy market mgr) │   │
                                    │   └──────┬───────┬──────────────┘   │
                                    │          ▼       ▼                  │
                                    │   ┌──────────┐ ┌──────────┐  ...    │
                                    │   │ Market   │ │ Market   │         │
                                    │   │ BTC_USDT │ │ ETH_USDT │         │
                                    │   │          │ │          │         │
                                    │   │  inbox   │ │  inbox   │         │
                                    │   │  book    │ │  book    │         │
                                    │   │  (own    │ │  (own    │         │
                                    │   │   goro)  │ │   goro)  │         │
                                    │   └────┬─────┘ └────┬─────┘         │
                                    └────────┼────────────┼───────────────┘
                                             │            │
                                             ▼            ▼
                                  ┌────────────────────────────┐
                                  │   Redis snapshot (proto)   │
                                  │   downstream publish hook  │
                                  └────────────────────────────┘
```

Each `Market` owns its order book outright. There is no shared state
between markets, so the matching path runs entirely lock-free per symbol.
The Registry's only synchronization is a coarse-grained mutex around the
market lookup map, taken only on the first message for a new symbol.

Inside a Market, the order book is:

- two sorted ladders (`Buy` descending, `Sell` ascending) of `[]*Level`
- a `map[price]*Level` per side for O(1) price-level lookup
- a `map[orderID]*OrderNode` for O(1) order lookup at cancel time
- a doubly-linked FIFO queue of orders at each level

This is the standard high-performance shape: cheap insert and cancel,
predictable behavior under both quiet and burst flow.

---

## Benchmarks

Repeatable numbers from `go test -bench=.` on a single core of a 13th-gen
Intel i7-13620H. Run them yourself with:

```bash
go test -run='^$' -bench=. -benchtime=2s ./matchEngine/...
```

| Benchmark | Phase 1 (ns/op) | Phase 2 (ns/op) | Speedup | Ops/sec/core |
|---|---:|---:|---:|---:|
| `MatchAtBest`        |  4,412 |     438.7 |   **10.1×** |  2.28M |
| `AddNonCrossing`     |  9,920 |     419.5 |   **23.6×** |  2.38M |
| `SweepFiveLevels`    | 55,005 |   8,198   |    **6.7×** |  122K  |
| `CancelMiss`         | 18,882 |      12.7 | **1,482×**  | 78.5M  |
| `CancelHit` (add+cancel pair) |    —   |     411.7 |        —    | 2.43M (op pair) |
| `EndToEndMixed`      | 203,064 |     425.6 |   **477×**  |  2.35M |

The mixed workload — 70% post liquidity, 20% take (cross), 10% cancel, at
~2,000 resting orders across 200 price levels — is the closest single
number to "what a busy market actually looks like". Phase 1 ran it at
~5,000 ops/sec; Phase 2 hits **~2.35M ops/sec per symbol per core**.

Numbers are per *single goroutine on a single symbol*. Each additional
active symbol takes its own core, so a 16-core box handling 16 hot symbols
delivers roughly **37M end-to-end ops/sec aggregate**.

Full benchmark output: [`bench/phase-2-final.txt`](bench/phase-2-final.txt)
(current) and [`bench/baseline-phase1.txt`](bench/baseline-phase1.txt)
(comparison anchor).

### Latency percentiles

Average throughput tells you the typical cost; for an exchange the question
that matters to operators is "how slow is your worst order?". Latency
percentiles, measured over 200,000 mixed operations after a 5,000-op
warmup, with one stop-the-world GC immediately before measurement:

| Metric | Value |
|---|---:|
| Avg     | 578 ns |
| P50     | 344 ns |
| P90     | 605 ns |
| P99     | 5.4 µs |
| P99.9   | 49 µs |
| P99.99  | 94 µs |
| Max     | 457 µs |

Reproduce with:

```bash
go test -run=TestLatencyPercentiles -v ./matchEngine/...
```

Full report: [`bench/phase-2-latency.txt`](bench/phase-2-latency.txt).

### Where the wins came from

Phase 2 was a four-step engineering pass; each step was independently
shippable and verified against the same benchmarks.

| Step | Change | What it bought |
|---|---|---|
| 2a + 2b | Replaced `shopspring/decimal` in the hot path with 8-decimal scaled `int64` (`Px` / `Qty`) | 5–9× across the board; every price compare became a single CPU instruction |
| 2c + 2d | Doubly-linked FIFO per level + `map[price]*Level` + `map[orderID]*OrderNode` | Cancel went from O(n·m) walk to O(1) lookup; insert from O(n) scan to O(log n) binary-searched position |
| 2e | One goroutine per symbol, no shared mutable state | Cross-symbol concurrency became genuine; the old shared-mutex registry is gone |

---

## Roadmap

- **Phase 1 — Credibility.** *(✅ shipped)* Correctness fixes, race-safe
  per-symbol locking, FIFO at each price level, real benchmarks, CI,
  Apache 2.0 license.
- **Phase 2 — Performance.** *(✅ shipped)* Scaled int64 fixed-point in
  the hot path, price-indexed ladder, O(1) cancel, per-symbol sharding,
  P50/P99/P99.9 latency tracking. Achieved ~2.35M ops/sec/core on the
  mixed workload — 477× faster than the Phase 1 baseline.
- **Phase 3 — Production features.** Self-trade prevention, IOC / FOK /
  post-only time-in-force flags, order modify, L2 market-data publishing,
  sequence numbers, snapshot + journal-based recovery, audit trail.
- **Phase 4 — Operability.** Prometheus metrics, OpenTelemetry tracing,
  graceful shutdown, replay tooling, fuzz harness for the matching loop.

---

## Quick start

Requirements: Go 1.24+, Docker.

```bash
# bring up Kafka + Redis
docker compose up -d

# run the engine
SUPPORTED_SYMBOLS=BTC_USDT,ETH_USDT go run .

# in another terminal — push a synthetic load
cd loadTest && go run .
```

The engine consumes from the Kafka topic `main_topic`. Order messages are
JSON-encoded:

```json
{
  "id": 1,
  "symbol": "BTC_USDT",
  "side": "buy",
  "type": "limit",
  "price": "10000",
  "volume": "1.5"
}
```

The Kafka message **key** controls the command:

| Key prefix      | Action |
|---|---|
| `create_<id>`   | Submit new order  |
| `cancel_<id>`   | Cancel existing order |
| `start_loadtest`| Begin load-test timer |
| `end_loadtest`  | End load-test timer (logs elapsed) |

Order prices and volumes accept either JSON strings (`"10000.50"`,
preferred — round-trips without precision loss) or JSON numbers
(`10000.50`). Internally everything is scaled-int64 with 8 fractional
decimal digits.

---

## Order types

- **Limit** — rests on the book until matched or cancelled.
- **Market** — matches against the best available prices and drops any
  unfilled remainder.
- **Stop-limit** — held off-book until the trigger price is observed,
  then promoted to a limit order. Re-evaluated on every match.

---

## Testing

```bash
go test -race ./...                          # all unit tests, race detector on
go test -run='^$' -bench=. ./matchEngine/... # throughput benchmarks
go test -run=Latency -v ./matchEngine/...    # latency percentile report
```

Unit tests cover the matching loop, regression tests pin every bug caught
during the Phase 1 audit, market tests verify cross-symbol isolation and
ordering, and the latency test produces a P50/P99/P99.9 report on every
run.

---

## Project layout

```
.
├── main.go                 # process entry: Kafka consumer + dispatch loop
├── kafka.go                # consumer wiring
├── instruments.go          # Kafka command/Instrument types
├── messaging.go            # downstream publishing hook
├── market/                 # per-symbol Market + Registry (sharded execution)
│   ├── market.go
│   ├── registry.go
│   └── market_test.go
├── matchEngine/            # matching loop dispatch + benchmarks
│   ├── matchEngine.go
│   ├── matchEngine_test.go
│   ├── matchEngine_regression_test.go
│   ├── matchEngine_bench_test.go
│   └── matchEngine_latency_test.go
├── models/                 # Order / Orderbook / Match / Px / Qty + linked list
├── persistance/            # Redis snapshot
├── utils/                  # env helpers, race-safe mutex registry
├── bench/                  # baseline benchmark outputs
└── loadTest/               # end-to-end Kafka load generator
```

---

## Contributing

Issues and pull requests welcome. Please run

```bash
go test -race ./... && go vet ./...
```

before submitting. CI runs `golangci-lint`, race tests, and a benchmark
smoke test on every PR.

---

## License

[Apache License 2.0](LICENSE). Copyright 2026 Ali Ahmadi.

---

## About the author

Gun is built and maintained by **Ali Ahmadi** — senior software engineer
focused on fintech infrastructure and low-latency systems in Go. Previous
work includes order/cancel latency and matching-engine throughput
improvements at production crypto exchanges.

Reach out for matching-engine, exchange-infra, or low-latency Go consulting:

- GitHub: [@aliraad79](https://github.com/aliraad79)
- Email: [dev@raastin.com](mailto:dev@raastin.com)
