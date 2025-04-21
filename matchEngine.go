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

	remainVolume := newOrder.Volume

	if newOrder.Side == BUY {
	outerloop:
		for _, matchEngineEntry := range orderbook.sell {
			if newOrder.Price >= matchEngineEntry.price {
				for i, order := range matchEngineEntry.orders {
					if remainVolume <= 0 {
						break outerloop
					}
					matches = append(matches, Match{BuyId: newOrder.ID, SellId: order.ID, MatchPrice: matchEngineEntry.price})
					if remainVolume >= order.Volume {
						matchEngineEntry.orders = matchEngineEntry.orders[i:]
					}
					remainVolume -= order.Volume
				}
			}
		}
		if remainVolume > 0 {

		}

	} else if newOrder.Side == SELL {
	outerloop2:
		for _, matchEngineEntry := range orderbook.buy {
			if newOrder.Price >= matchEngineEntry.price {
				for i, order := range matchEngineEntry.orders {
					if remainVolume <= 0 {
						break outerloop2
					}
					matches = append(matches, Match{BuyId: order.ID, SellId: newOrder.ID, MatchPrice: matchEngineEntry.price})
					if remainVolume >= order.Volume {
						matchEngineEntry.orders = matchEngineEntry.orders[i:]
					}
					remainVolume -= order.Volume
				}
			}
		}
	}

	if remainVolume > 0 {
		orderbook.add(newOrder)
	}
	return matches
}
