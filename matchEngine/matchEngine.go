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
