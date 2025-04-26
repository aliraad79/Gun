# Gun

**Modern high-performance match engine**

---

## Design Decisions

### High Throughput
- Should handle **5k orders per market per second**.
- Matches must be published **within one second** if the incoming order rate is ~5k messages per second. which lead to **435 Mil order** in a day
- Take in mind that this number is for just one market and applying this match engine on multiple markets considering the options that is available in go lang shouldn't add time in linear order
### Continuous Availability
- No need to restart the engine when:
  - Adding a new orderbook.
  - Stopping an existing orderbook.

---

## Load Testing

To run a load test:

```bash
cd loadTest
go run .
```

---

## Topics

- Creating and cancelling an order → `main_topic`

---

## Complexity

- **Create a new order:**  
  `O(n)`, where `n` is the number of price levels on the order side.
  
- **Cancel an order:**  
  `O(nm)`, where:
  - `n` = number of price levels on the order side.
  - `m` = number of orders at that price level.

> **Note:** Both operations can be optimized to `O(log n)` using binary search.

---

## Storage

- The match engine is **memory-based** and save the data in a persistance in memory storage(for now it is redis).

---

## Assumptions

- The producer knows the sequence of id and don't send multiple ids

---

## Tests

on a laptop which has
- CPU: 11th Gen Intel i7-1165G7 (8) @ 4.700GHz
- Memory: 15776MiB
Load test ended in 14.190214056s for 5k messages
