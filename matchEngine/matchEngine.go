package matchEngine

import (
	"fmt"

	"errors"
	"os"
	"strings"

	. "github.com/aliraad79/Gun/data"
	"github.com/aliraad79/Gun/persistance"
	utils "github.com/aliraad79/Gun/utils"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

func HandleConditionalOrders(lastMatchPrice decimal.Decimal) {
	log.Info("handeling conditional orders based on ", lastMatchPrice)
}

func AddNewOrder(orderbook *Orderbook, newOrder Order) []Match {
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
		orderbook.Add(newOrder)
	}
	return matches
}

func CancelOrder(orderbook *Orderbook, newOrder Order) []Match {
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
		orderbook.Add(newOrder)
	}
	return matches
}

var ErrNotValidSymbol = errors.New("item not found")

func createOrderbooks(symbol string) (*Orderbook, error) {
	supported_symbols := os.Getenv("SUPPORTED_SYMBOLS")
	symbols := strings.Split(supported_symbols, ",")

	if utils.Contains(symbols, symbol) {
		return &Orderbook{Symbol: symbol}, nil
	}
	return nil, ErrNotValidSymbol
}

func LoadOrFetchOrderbook(memory map[string]*Orderbook, symbol string) (*Orderbook, error) {
	_, exists := memory[symbol]
	if exists {
		return memory[symbol], nil
	} else {
		var err error

		orderbook := persistance.LoadOrderbook(symbol)

		if orderbook == nil {
			orderbook, err = createOrderbooks(symbol)
		}
		log.Warn("pp ", orderbook)
		memory[symbol] = orderbook
		return orderbook, err
	}
}
