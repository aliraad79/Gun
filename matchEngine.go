package main

import (
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

func handleConditionalOrders(lastMatchPrice decimal.Decimal) {
	log.Info("handeling conditional orders based on ", lastMatchPrice)
}

func processOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match

	remainVolume := newOrder.Volume

	if newOrder.Side == BUY {
		for _, matchEngineEntry := range orderbook.Sell {
			if newOrder.Price.GreaterThanOrEqual(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if remainVolume.LessThanOrEqual(decimal.Zero) {
						break
					}
					matches = append(matches, Match{BuyId: newOrder.ID, SellId: order.ID, MatchPrice: matchEngineEntry.Price})
					if remainVolume.GreaterThanOrEqual(order.Volume) {
						matchEngineEntry.Orders = matchEngineEntry.Orders[i:]
					}
					remainVolume = remainVolume.Sub(order.Volume)
				}
			}
		}
	} else if newOrder.Side == SELL {
		for _, matchEngineEntry := range orderbook.Buy {
			if newOrder.Price.GreaterThanOrEqual(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if remainVolume.LessThanOrEqual(decimal.Zero) {
						break
					}
					matches = append(matches, Match{BuyId: order.ID, SellId: newOrder.ID, MatchPrice: matchEngineEntry.Price})
					if remainVolume.GreaterThanOrEqual(order.Volume) {
						matchEngineEntry.Orders = matchEngineEntry.Orders[i:]
					}
					remainVolume = remainVolume.Sub(order.Volume)
				}
			}
		}
	}

	if remainVolume.GreaterThan(decimal.Zero) {
		orderbook.add(newOrder)
	}
	return matches
}
