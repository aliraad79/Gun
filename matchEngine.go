package main

import "log"

func handleConditionalOrders(lastMatchPrice float64) {
	log.Println("handeling conditional orders based on", lastMatchPrice)
}
func processOrders(orderbook Orderbook, order Order) []Match {
	return matchOrder(orderbook, order)
}

func matchOrder(orderbook Orderbook, newOrder Order) []Match {
	var matches []Match

	remainVolume := newOrder.volume

	if newOrder.side == BUY {
	outerloop:
		for _, matchEngineEntry := range orderbook.sell {
			if newOrder.price >= matchEngineEntry.price {
				for i, order := range matchEngineEntry.orders {
					if remainVolume <= 0 {
						break outerloop
					}
					matches = append(matches, Match{buy_id: newOrder.id, sell_id: order.id, match_price: matchEngineEntry.price})
					if remainVolume >= order.volume {
						matchEngineEntry.orders = matchEngineEntry.orders[i:]
					}
					remainVolume -= order.volume
				}
			}
		}
		if remainVolume > 0 {

		}

	} else if newOrder.side == SELL {
	outerloop2:
		for _, matchEngineEntry := range orderbook.buy {
			if newOrder.price >= matchEngineEntry.price {
				for i, order := range matchEngineEntry.orders {
					if remainVolume <= 0 {
						break outerloop2
					}
					matches = append(matches, Match{buy_id: order.id, sell_id: newOrder.id, match_price: matchEngineEntry.price})
					if remainVolume >= order.volume {
						matchEngineEntry.orders = matchEngineEntry.orders[i:]
					}
					remainVolume -= order.volume
				}
			}
		}
	}

	if remainVolume > 0 {
		orderbook.add(newOrder)
	}
	return matches
}
