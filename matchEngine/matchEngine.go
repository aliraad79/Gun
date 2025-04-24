package matchEngine

import (
	"fmt"

	"errors"
	"os"
	"strings"

	. "github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	utils "github.com/aliraad79/Gun/utils"
	"github.com/shopspring/decimal"

	log "github.com/sirupsen/logrus"
)

func HandleConditionalOrders(orderbook *Orderbook, lastMatches []Match) []Match {
	var matches []Match

	for _, order := range orderbook.ConditionalOrders {
		for _, match := range lastMatches {
			if order.IsTriggered(match.Price) {
				newMatches := handleLimitOrder(orderbook, order)
				matches = append(matches, newMatches...)
			}
		}
	}

	return matches
}

func MatchAndAddNewOrder(orderbook *Orderbook, newOrder Order) []Match {
	switch newOrder.Type {
	case LIMIT:
		return handleLimitOrder(orderbook, newOrder)
	case MARKET:
		return handleMarketOrder(orderbook, newOrder)
	case STOP_lIMIT:
		return handleStopLimitOrder(orderbook, newOrder)
	default:
		panic(fmt.Sprintf("unexpected models.Type: %#v", newOrder.Type))
	}
}

func handleLimitOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match

	remainVolume := newOrder.Volume

loop:
	switch newOrder.Side {
	case BUY:
		for idx, matchEngineEntry := range orderbook.Sell {
			// i know this is tricky but for now it do the work
			for i := len(matchEngineEntry.Orders) - 1; i >= 0; i-- {
				order := matchEngineEntry.Orders[i]
				if remainVolume.LessThanOrEqual(decimal.Zero) {
					break loop
				}
				matchCandidate := Match{BuyId: newOrder.ID, SellId: order.ID, Price: matchEngineEntry.Price}
				if remainVolume.GreaterThanOrEqual(order.Volume) {
					orderbook.Sell[idx].Orders = append(orderbook.Sell[idx].Orders[i:], orderbook.Sell[idx].Orders[i+1:]...)
					matchCandidate.Volume = order.Volume
				} else {
					orderbook.Sell[idx].Orders[i].Volume = orderbook.Sell[idx].Orders[i].Volume.Sub(remainVolume)
					matchCandidate.Volume = remainVolume
				}
				remainVolume = remainVolume.Sub(order.Volume)
				matches = append(matches, matchCandidate)
			}
		}
	case SELL:
		for idx, matchEngineEntry := range orderbook.Buy {
			for i := len(matchEngineEntry.Orders) - 1; i >= 0; i-- {
				order := matchEngineEntry.Orders[i]

				if remainVolume.LessThanOrEqual(decimal.Zero) {
					break loop
				}
				matchCandidate := Match{BuyId: order.ID, SellId: newOrder.ID, Price: matchEngineEntry.Price}
				if remainVolume.GreaterThanOrEqual(order.Volume) {
					orderbook.Buy[idx].Orders = append(orderbook.Buy[idx].Orders[i:], orderbook.Buy[idx].Orders[i+1:]...)
					matchCandidate.Volume = order.Volume
				} else {
					orderbook.Buy[idx].Orders[i].Volume = orderbook.Buy[idx].Orders[i].Volume.Sub(remainVolume)
					matchCandidate.Volume = remainVolume
				}
				remainVolume = remainVolume.Sub(order.Volume)
				matches = append(matches, matchCandidate)
			}
		}
	default:
		log.Warn("unexpected main.Side. ", newOrder.Side)
		return nil
	}

	if remainVolume.GreaterThan(decimal.Zero) {
		orderbook.Add(newOrder)
	}

	return matches
}

func handleMarketOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match

	remainVolume := newOrder.Volume

loop:
	switch newOrder.Side {
	case BUY:
		for idx, matchEngineEntry := range orderbook.Sell {
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
		log.Warn("unexpected main.Side. ", newOrder.Side)
		return nil
	}

	if remainVolume.GreaterThan(decimal.Zero) {
		log.Warn("UNfinished market order. drop the order. ", newOrder)
	}

	return matches
}

func handleStopLimitOrder(orderbook *Orderbook, order Order) []Match {
	//todo: Must get this from some memory
	lastPrice := decimal.Zero
	if order.IsTriggered(lastPrice) {
		return handleLimitOrder(orderbook, order)
	}
	orderbook.AddConditionalOrder(order)
	return nil
}

var ErrCancelOrderFailed = errors.New("cancelling order failed")

func CancelOrder(orderbook *Orderbook, targetOrder Order) error {

	switch targetOrder.Side {
	case BUY:
		for idx, matchEngineEntry := range orderbook.Buy {
			if targetOrder.Price.Equal(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if order.ID == targetOrder.ID {
						orders := matchEngineEntry.Orders

						orderbook.Buy[idx].Orders = append(orders[:i], orders[i+1:]...)
						return nil
					}
				}
			}
		}
	case SELL:
		for idx, matchEngineEntry := range orderbook.Sell {
			if targetOrder.Price.GreaterThanOrEqual(matchEngineEntry.Price) {
				for i, order := range matchEngineEntry.Orders {
					if order.ID == targetOrder.ID {
						orders := matchEngineEntry.Orders

						orderbook.Sell[idx].Orders = append(orders[:i], orders[i+1:]...)
						return nil
					}
				}
			}
		}
	default:
		panic(fmt.Sprintf("unexpected main.Side: %#v", targetOrder.Side))
	}

	return ErrCancelOrderFailed
}

var ErrNotValidSymbol = errors.New("item not found")

func createOrderbook(symbol string) (*Orderbook, error) {
	supported_symbols := os.Getenv("SUPPORTED_SYMBOLS")
	symbols := strings.Split(supported_symbols, ",")

	if utils.Contains(symbols, symbol) {
		return &Orderbook{Symbol: symbol}, nil
	}
	return nil, ErrNotValidSymbol
}

func LoadOrFetchOrderbook(symbol string) (*Orderbook, error) {
	orderbook := persistance.LoadOrderbook(symbol)

	if orderbook == nil {
		return createOrderbook(symbol)
	}
	return orderbook, nil
}
