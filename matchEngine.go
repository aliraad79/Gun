package main

import (
	"fmt"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

func handleConditionalOrders(lastMatchPrice decimal.Decimal) {
	log.Info("handeling conditional orders based on ", lastMatchPrice)
}

func processOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match

	remainVolume := newOrder.Volume

loop:
	switch newOrder.Side {
	case BUY:
		for idx, matchEngineEntry := range orderbook.Sell {
			if newOrder.Price.GreaterThanOrEqual(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if remainVolume.LessThanOrEqual(decimal.Zero) {
						break loop
					}
					matchCandidate := Match{BuyId: newOrder.ID, SellId: order.ID, Price: matchEngineEntry.Price}
					if remainVolume.GreaterThanOrEqual(order.Volume) {
						orderbook.Sell[idx].Orders = orderbook.Sell[idx].Orders[i:]
						matchCandidate.Volume = order.Volume
					} else {
						matchCandidate.Volume = remainVolume
					}
					remainVolume = remainVolume.Sub(order.Volume)
					matches = append(matches, matchCandidate)
				}
			}
		}
	case SELL:
		for idx, matchEngineEntry := range orderbook.Buy {
			if newOrder.Price.GreaterThanOrEqual(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if remainVolume.LessThanOrEqual(decimal.Zero) {
						break loop
					}
					matchCandidate := Match{BuyId: order.ID, SellId: newOrder.ID, Price: matchEngineEntry.Price}
					if remainVolume.GreaterThanOrEqual(order.Volume) {
						orderbook.Buy[idx].Orders = orderbook.Buy[idx].Orders[i:]
						matchCandidate.Volume = order.Volume
					} else {
						matchCandidate.Volume = remainVolume
					}
					remainVolume = remainVolume.Sub(order.Volume)
					matches = append(matches, matchCandidate)
				}
			}
		}
	default:
		panic(fmt.Sprintf("unexpected main.Side: %#v", newOrder.Side))
	}

	if remainVolume.GreaterThan(decimal.Zero) {
		orderbook.add(newOrder)
	}
	return matches
}
