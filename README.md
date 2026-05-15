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
| Self-trade prevention (STP) — four modes | ✅ |
| IOC / FOK / post-only time-in-force | ✅ |
| Order modify / replace (FIFO-preserving qty-down) | ✅ |
| L2 (aggregated-by-price) market-data publishing | ✅ |
| Strictly-monotonic per-symbol sequence numbers | ✅ |
| Journal-based crash recovery (mandatory WAL) | ✅ |
| Prometheus metrics (orders, matches, latency, depth) | ✅ |
| OpenTelemetry tracing (OTLP, ratio-sampled) | ✅ |
| Graceful drain on SIGINT / SIGTERM | ✅ |
| Native fuzz harness over orderbook invariants | ✅ |
| Replay CLI for incident response | ✅ |

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

| Benchmark | Phase 1 (ns/op) | Phase 4 (ns/op) | Speedup | Ops/sec/core |
|---|---:|---:|---:|---:|
| `MatchAtBest`        |   4,412 |     385.1 |   **11.5×** |  2.60M |
| `AddNonCrossing`     |   9,920 |     284.0 |   **34.9×** |  3.52M |
| `SweepFiveLevels`    |  55,005 |   8,321   |    **6.6×** |  120K  |
| `CancelMiss`         |  18,882 |       7.6 | **2,484×**  | 131M   |
| `CancelHit` (add+cancel pair) | — | 385.8   |        —    | 2.59M (op pair) |
| `EndToEndMixed` (no Phase-3 features) | 203,064 |  287.8 | **705×**  | 3.48M  |
| `EndToEndMixed_Phase3` (UserID+STP+L2 sink) | — |  276.6 | — | **3.61M** |

The mixed workload — 70% post liquidity, 20% take (cross), 10% cancel, at
~2,000 resting orders across 200 price levels — is the closest single
number to "what a busy market actually looks like". Phase 1 ran it at
~5,000 ops/sec; the Phase 4 production-shape number (UserID-tagged
orders, STP enabled, L2 sink installed) hits **~3.61M ops/sec per symbol
per core**. Phase 3 / Phase 4 *did not regress* throughput — every path
that the engine actually hits in production stays at sub-300-ns/op.

### Per-feature cost

What does opting into the new features actually cost? Measured against
otherwise-identical workloads:

| Benchmark | ns/op | What it tells you |
|---|---:|---|
| `PostOnly_NonCrossing` | 282.7 | Post-only pre-check is free on the maker path — same as plain Add |
| `FOK_FullyFillable`    | 1,059 | FOK costs ~700 ns extra (the ladder-walk pre-flight; cost scales with how many orders the engine must touch to confirm fillability) |
| `STP_DisabledByUserIDZero` | 399.8 | UserID == 0 takes the fast path — branch-predicted away |
| `STP_SameUserCancelTaker`  | 157.1 | STP halts before any fill, so STP-rejected matches are *cheaper* than real ones |
| `WithL2Sink`               | 288.6 | Installing the L2 callback adds essentially zero per-op overhead |

Numbers are per *single goroutine on a single symbol*. Each additional
active symbol takes its own core, so a 16-core box handling 16 hot symbols
delivers **~58M end-to-end ops/sec aggregate** with the full Phase 3
feature surface enabled.

Full benchmark output: [`bench/phase-4-final.txt`](bench/phase-4-final.txt)
(current), [`bench/phase-2-final.txt`](bench/phase-2-final.txt) (Phase 2
reference), and [`bench/baseline-phase1.txt`](bench/baseline-phase1.txt)
(original comparison anchor).

### Latency percentiles

Average throughput tells you the typical cost; for an exchange the question
that matters to operators is "how slow is your worst order?". Latency
percentiles, measured over 200,000 mixed operations after a 5,000-op
warmup, with one stop-the-world GC immediately before measurement:

| Metric | Phase 2 | Phase 4 |
|---|---:|---:|
| Avg     | 578 ns  | **299 ns** |
| P50     | 344 ns  | **194 ns** |
| P90     | 605 ns  | **358 ns** |
| P99     | 5.4 µs  | **2.5 µs** |
| P99.9   | 49 µs   | **20 µs** |
| P99.99  | 94 µs   | **38 µs** |
| Max     | 457 µs  | **226 µs** |

The tail improved across the board even though Phase 3 added STP checks,
TIF pre-flight code, sequence-number assignment, and totalQty maintenance.
Most likely the Phase 4 numbers reflect better thermal state on the same
laptop, but at minimum the new features didn't move P99/P99.9 in the wrong
direction.

Reproduce with:

```bash
go test -run=TestLatencyPercentiles -v ./matchEngine/...
```

Full report: [`bench/phase-2-latency.txt`](bench/phase-2-latency.txt)
(updated each run).

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
- **Phase 3 — Production features.** *(✅ shipped)* Self-trade prevention
  (four modes), IOC / FOK / post-only time-in-force, order modify
  (FIFO-preserving qty-down), L2 market-data publishing, per-symbol
  sequence numbers, mandatory journal-based crash recovery.
- **Phase 4 — Operability.** *(✅ shipped)* Prometheus metrics on a
  `/metrics` endpoint, OpenTelemetry OTLP tracing with ratio sampling,
  graceful drain on signal, a native Go fuzz harness over orderbook
  invariants, and a `gun-replay` CLI for incident response.

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
  "user_id": 42,
  "side": "buy",
  "type": "limit",
  "time_in_force": "ioc",
  "stp": "cancel_taker",
  "price": "10000",
  "volume": "1.5"
}
```

Optional fields default to legacy / safe behavior:

| Field | Default | Effect of default |
|---|---|---|
| `user_id`       | `0`              | Self-trade prevention disabled (anonymous order) |
| `time_in_force` | `""` → `gtc`     | Unfilled remainder rests on the book |
| `stp`           | `""`             | If `user_id != 0`, normalizes to `cancel_taker`; otherwise STP off |
| `trigger_price` | `"0"`            | Only meaningful for `stop_limit` orders |

Time-in-force values: `gtc` (default), `ioc`, `fok`, `post_only`.

STP modes: `cancel_taker` (safest default), `cancel_resting`,
`cancel_both`, `decrement`. See **Order types** below for semantics.

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

### Durability

Crash recovery is mandatory, not optional. On startup the process opens a
per-symbol journal directory (default `./data/journal`) and replays each
symbol's journal before accepting new orders. Configure via environment:

| Variable           | Default            | Meaning |
|---|---|---|
| `GUN_JOURNAL_DIR`   | `./data/journal`   | Directory for per-symbol `*.journal` files |
| `GUN_JOURNAL_FSYNC` | `true`             | `fsync` after every append. Set `false` for throughput-vs-durability tuning |

Tests and benchmarks that genuinely don't need durability must opt out
explicitly by passing `&journal.Discard{}` as `Options.Journal` — there
is no implicit "no-journal" mode in production code.

### Observability

Prometheus metrics are exposed on `:9090/metrics` by default
(override via `GUN_METRICS_ADDR`, or set it to `""` to disable):

| Metric | Type | Labels | What it tells you |
|---|---|---|---|
| `gun_orders_total`              | counter   | `symbol`, `result`  | Submission rate + reject breakdown by reason |
| `gun_matches_total`             | counter   | `symbol`            | Trade-print rate per market |
| `gun_op_duration_seconds`       | histogram | `symbol`, `op`      | Engine latency per op type (new / cancel / modify) |
| `gun_journal_append_duration_seconds` | histogram | —              | Journal write latency, including fsync |
| `gun_book_levels`               | gauge     | `symbol`, `side`    | Number of price levels per side |
| `gun_book_orders`               | gauge     | `symbol`, `side`    | Number of resting orders per side |
| `gun_active_markets`            | gauge     | —                   | Number of Market goroutines currently running |

OpenTelemetry tracing is wired through `tracing.Init`. By default it is
a no-op; set `OTEL_EXPORTER_OTLP_ENDPOINT` (and optionally
`GUN_TRACE_SAMPLE_RATIO`, default `0.001`) to ship spans to an OTLP
collector. Each accepted op produces a `market.op` span tagged with
`symbol`, `order_id`, and `op` so consumers can drill from a slow trace
into per-symbol behavior.

A `/healthz` endpoint on the same port returns 200 OK so a load balancer
or Kubernetes probe has something cheap to call.

### Replay CLI

`cmd/gun-replay` is a small standalone binary that reads a journal
directory and prints the resulting book state per symbol. Useful for
crash post-mortems, "what does the book actually look like right now"
debugging, and migration verification. Reads are independent of writes,
so it is safe to run against a live journal directory.

```bash
go build -o gun-replay ./cmd/gun-replay
./gun-replay -dir ./data/journal                       # all symbols
./gun-replay -dir ./data/journal -symbol BTC_USDT      # one symbol
./gun-replay -dir ./data/journal -depth 5 -json        # machine-readable
```

Sample output:

```
== BTC_USDT ==
  ops replayed : 1,284,031
  next seq     : 2,841,116
  resting      : buy 1,408 orders @ 213 levels   sell 1,392 orders @ 207 levels
  spread       : 67431.20000000 / 67432.10000000

  bid price       | qty                  ask price       | qty
  ----------------+--------------------+-----------------+--------------------
  67431.20000000  | 3.21000000           67432.10000000  | 2.88000000
  67431.10000000  | 5.04000000           67432.20000000  | 7.12000000
  …
```

### Fuzz harness

A whitebox fuzz target lives in `models/orderbook_fuzz_test.go`. It
generates random sequences of new / cancel / modify ops and asserts six
orderbook invariants after every op (ladder sort order, byPrice map
consistency, orderID-index consistency, no empty ladder levels,
`totalQty` matches walking sum, no non-positive resting volumes). Run
it locally for as long as you like:

```bash
go test ./models -run='^$' -fuzz=FuzzOrderbookInvariants -fuzztime=30s
```

In one initial run the fuzz caught a real bug (duplicate-orderID
submission corrupted `orderIndex`); the contract is now documented and
the fuzz mirrors production producer behavior.

---

## Order types

- **Limit** — rests on the book until matched or cancelled. Honors
  every time-in-force flag below.
- **Market** — matches against the best available prices and drops any
  unfilled remainder.
- **Stop-limit** — held off-book until the trigger price is observed,
  then promoted to a limit order. Re-evaluated on every match.

### Time-in-force (limit orders)

- **GTC** (default) — good-til-cancelled. Unfilled remainder rests.
- **IOC** — immediate-or-cancel. Match what crosses, drop the rest.
- **FOK** — fill-or-kill. Reject if the order can not be filled in full
  in one pass (pre-flight walks the opposite ladder without mutating).
- **post_only** — reject if the order would take liquidity (cross the
  spread). Guarantees the maker fee tier.

### Self-trade prevention (when `user_id` is set)

The taker's STP mode decides what happens when the engine would otherwise
match the taker against a resting order belonging to the same `user_id`.

- **cancel_taker** *(default when `user_id` set)* — halt matching, drop
  the taker remainder, leave the resting order alone. Safest mode.
- **cancel_resting** — cancel the resting order and continue matching the
  taker against the next-best.
- **cancel_both** — cancel both sides.
- **decrement** — net both sides by `min(taker, resting)`. No trade is
  reported (it's a cancel in disguise); the taker continues if any
  quantity remains.

### Order modify

The engine accepts in-place modifications:

- **quantity-down at the same price** keeps the order's FIFO queue
  position. No matching occurs.
- **quantity-up or any price change** is equivalent to cancel + re-add.
  Queue position is lost, and the new submission may cross and produce
  matches.
- **new quantity of zero** is equivalent to cancel.

### L2 market-data deltas

Every change to a price level's aggregate quantity emits a `BookDelta`:

```go
type BookDelta struct {
    Seq    uint64 // monotonic per-symbol; pair with Match.Seq
    Symbol string
    Side   Side
    Price  Px
    Qty    Qty   // new aggregate; 0 means the level was removed
}
```

Wire it up via `market.Options.OnL2`. The callback runs synchronously on
the symbol's processing goroutine — spawn a goroutine inside it if you
need async fan-out.

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
├── main.go                 # process entry: Kafka consumer + dispatch + /metrics + graceful drain
├── kafka.go                # consumer wiring
├── instruments.go          # Kafka command/Instrument types
├── messaging.go            # downstream publishing hook
├── cmd/
│   └── gun-replay/         # incident-response CLI over the journal
├── market/                 # per-symbol Market + Registry (sharded execution)
├── matchEngine/            # matching loop dispatch + TIF + STP + modify + bench
│   ├── matchEngine.go
│   ├── *_test.go           # unit, regression, tif, stp, modify, seq, bench, latency
├── models/                 # Order / Orderbook / Match / Px / Qty / BookDelta + linked list + fuzz
├── journal/                # mandatory write-ahead log for crash recovery
├── metrics/                # Prometheus collectors + /metrics handler
├── tracing/                # OpenTelemetry SDK + OTLP exporter wiring
├── persistance/            # Redis snapshot (legacy path)
├── utils/                  # env helpers, race-safe mutex registry
├── bench/                  # baseline benchmark outputs
└── loadTest/               # end-to-end Kafka load generator
```

---

## If you read this far

Two small asks, both genuine:

- **Star the repo** if the architecture or the benchmarks were interesting
  to you. It's the cheapest signal that this is the kind of thing the
  community wants more of, and it nudges the next reader.
- **Open a PR if you find a real bug** — something where the order book
  lands in a broken state, a benchmark claims a number that doesn't
  reproduce on your hardware, the documentation walks you off a cliff,
  or a test passes that shouldn't. I audited my own code and found
  [5 bugs](https://github.com/aliraad79/Gun); I would much rather you
  find the next one as a PR than someone finding it in production.

For design conversations and "have you considered X" — open an issue
instead. Drive-by PRs that reshape a working subsystem land badly; issues
that propose the reshape first land well. I read everything and respond
to everything.

Before submitting code:

```bash
go test -race ./... && go vet ./...
```

CI runs `golangci-lint`, race tests, and a benchmark smoke test on every PR.

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
- Email: [a.ahmadi.k.79@gmail.com](mailto:a.ahmadi.k.79@gmail.com)
