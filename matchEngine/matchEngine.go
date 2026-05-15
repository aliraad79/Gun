package matchEngine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	. "github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/persistance"
	utils "github.com/aliraad79/Gun/utils"
)

// HandleConditionalOrders walks the orderbook's conditional queue and
// promotes any orders whose trigger has been hit by one of the just-produced
// matches.
func HandleConditionalOrders(orderbook *Orderbook, lastMatches []Match) []Match {
	var matches []Match
	for _, order := range orderbook.ConditionalOrders {
		for _, match := range lastMatches {
			if order.IsTriggered(match.Price) {
				matches = append(matches, handleLimitOrder(orderbook, order)...)
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
	case STOP_LIMIT:
		return handleStopLimitOrder(orderbook, newOrder)
	default:
		panic(fmt.Sprintf("unexpected models.Type: %#v", newOrder.Type))
	}
}

func handleLimitOrder(orderbook *Orderbook, newOrder Order) []Match {
	matches, remain := orderbook.MatchTaker(newOrder, false)
	if remain.IsPositive() {
		newOrder.Volume = remain
		orderbook.Add(newOrder)
	}
	return matches
}

func handleMarketOrder(orderbook *Orderbook, newOrder Order) []Match {
	// Market orders never rest; any unfilled remainder is silently dropped.
	matches, _ := orderbook.MatchTaker(newOrder, true)
	return matches
}

func handleStopLimitOrder(orderbook *Orderbook, order Order) []Match {
	// TODO: source the reference price from the last-trade tracker.
	if order.IsTriggered(ZeroPx) {
		return handleLimitOrder(orderbook, order)
	}
	orderbook.AddConditionalOrder(order)
	return nil
}

// CancelOrder removes the order with the given ID from the book.
// Returns models.ErrCancelOrderFailed if no such order is resting.
func CancelOrder(orderbook *Orderbook, targetOrderID int64) error {
	return orderbook.Cancel(targetOrderID)
}

var ErrNotValidSymbol = errors.New("item not found")

func createOrderbook(symbol string) (*Orderbook, error) {
	supportedSymbols := os.Getenv("SUPPORTED_SYMBOLS")
	symbols := strings.Split(supportedSymbols, ",")
	if utils.Contains(symbols, symbol) {
		return NewOrderbook(symbol), nil
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
