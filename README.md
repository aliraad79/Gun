# Gun

> A modern, in-memory matching engine for spot markets, written in Go.

[![CI](https://github.com/aliraad79/Gun/actions/workflows/ci.yml/badge.svg)](https://github.com/aliraad79/Gun/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Go Reference](https://pkg.go.dev/badge/github.com/aliraad79/Gun.svg)](https://pkg.go.dev/github.com/aliraad79/Gun)

Gun is a continuous-trading limit order book matching engine. It accepts new
orders from Kafka, matches them against an in-memory order book with strict
price–time priority, persists order book state to Redis, and publishes
resulting trades downstream.

It is designed to be embedded as the matching core of a larger exchange
stack — alongside an API gateway, risk/credit checks, market-data fan-out,
and clearing. Gun owns *one* concern: matching, fast and correctly.

---

## Features

| Feature | Status |
|---|---|
| Limit, market, and stop-limit orders | ✅ |
| Price–time priority (FIFO at each level) | ✅ |
| Per-symbol concurrency isolation | ✅ |
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
                ┌──────────┐        ┌──────────────────────────────┐
   orders ────► │  Kafka   │ ─────► │        Gun (this repo)       │
                └──────────┘        │                              │
                                    │   ┌──────────────────────┐   │
                                    │   │  per-symbol shard    │   │
                                    │   │  ┌────────────────┐  │   │
                                    │   │  │ Order book     │  │   │
                                    │   │  │ (BUY / SELL    │  │   │
                                    │   │  │  price levels) │  │   │
                                    │   │  └────────────────┘  │   │
                                    │   └──────────────────────┘   │
                                    │            │                 │
                                    │            ▼                 │
                                    │       matches                │
                                    └────────────┬─────────────────┘
                                                 │
                                                 ▼
                                  ┌────────────────────────────┐
                                  │   Redis snapshot (proto)   │
                                  │   downstream publish hook  │
                                  └────────────────────────────┘
```

A single Gun process can host many symbols. Each symbol runs under its own
mutex so independent markets do not serialize against each other.

---

## Benchmarks

Real, repeatable numbers from `go test -bench=.` on a single core of a
13th-gen Intel i7-13620H. Run them yourself with:

```bash
go test -run='^$' -bench=. -benchtime=2s ./matchEngine/...
```

Baseline (Phase 1, post bug-fixes; full output in [`bench/baseline-phase1.txt`](bench/baseline-phase1.txt)):

| Benchmark | ns/op | ops/sec (single core) | What it measures |
|---|---:|---:|---|
| `MatchAtBest`        |  4,412 | ~226,600 | Taker that fully consumes one resting order at top of book |
| `AddNonCrossing`     |  9,920 | ~100,800 | Posting passive liquidity (no match), depth = 1,000 levels |
| `CancelMidBook`      | 18,882 |  ~52,960 | Cancel at a non-best price level, depth = 200 levels |
| `SweepFiveLevels`    | 55,005 |  ~18,180 | Aggressive taker sweeping 5 levels at once |
| `EndToEndMixed`      | 203,064 |  ~4,925 | 70% post / 20% cross / 10% cancel, depth = 200 levels, 2,000 orders |

Notes:
- Numbers are per *single goroutine on a single symbol*. Symbol sharding is
  linear (one independent goroutine per market), so a 16-core box handling
  16 active symbols multiplies these throughputs.
- The mixed workload is the realistic "what does a busy market actually look
  like" number. It is dominated by `shopspring/decimal` allocations on
  every price comparison — Phase 2 will replace this with scaled int64
  fixed-point and is expected to yield a >10× improvement.

The legacy Kafka-driven load test under `loadTest/` measures the *end-to-end
pipeline* including Kafka serialization and Redis writes; it is a
system-level smoke test, not an engine-level number. Engine speed is what
the `go test -bench` numbers above report.

---

## Roadmap

Gun is being developed in phases. Each phase is independently shippable.

- **Phase 1 — Credibility** *(current)*. Correctness fixes, race-safe
  per-symbol locking, proper FIFO at each price level, real benchmarks, CI,
  Apache 2.0 license.
- **Phase 2 — Performance**. Scaled int64 fixed-point in the hot path,
  price-indexed ladder (constant-time price lookup), per-symbol sharding,
  P50/P99/P99.9 latency tracking. Target: **100k+ orders/sec/symbol** on
  commodity hardware in the mixed workload.
- **Phase 3 — Production features**. Self-trade prevention, IOC / FOK /
  post-only time-in-force flags, order modify, L2 market-data publishing,
  sequence numbers, snapshot + journal-based recovery, audit trail.
- **Phase 4 — Operability**. Prometheus metrics, OpenTelemetry tracing,
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

---

## Order types

- **Limit** — rests on the book until matched or cancelled.
- **Market** — matches against the best available prices and drops any
  unfilled remainder.
- **Stop-limit** — held off-book until the trigger price is observed, then
  promoted to a limit order. Re-evaluated on every match.

---

## Testing

```bash
go test -race ./...                          # all unit tests, race detector on
go test -run='^$' -bench=. ./matchEngine/... # benchmarks
```

The matching loop is covered by both unit tests (`matchEngine_test.go`) and
regression tests for specific bugs that were caught and fixed during the
Phase 1 audit (`matchEngine_regression_test.go`).

---

## Project layout

```
.
├── main.go                 # process entry: Kafka consumer, worker pool
├── kafka.go                # consumer wiring
├── instruments.go          # command dispatch
├── messaging.go            # downstream publishing hook
├── matchEngine/            # the matching loop
│   └── matchEngine.go
├── models/                 # Order, Orderbook, Match (+ protobuf)
├── persistance/            # Redis snapshot
├── utils/                  # per-symbol mutex registry, env helpers
└── loadTest/               # end-to-end Kafka load generator
```

---

## Contributing

Issues and pull requests are welcome. Please run

```bash
go test -race ./... && go vet ./...
```

before submitting. CI will also run `golangci-lint` and a benchmark smoke
test on every PR.

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
