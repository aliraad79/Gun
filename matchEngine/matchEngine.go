package matchEngine

import (
	"fmt"
	"slices"

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
	case STOP_LIMIT:
		return handleStopLimitOrder(orderbook, newOrder)
	default:
		panic(fmt.Sprintf("unexpected models.Type: %#v", newOrder.Type))
	}
}

// matchAgainstBook walks the opposite-side book in price-time priority and
// fills as much of taker as the crossing levels allow. The book is mutated
// in place: resting orders are reduced or removed; price levels are removed
// when they empty out. Returns the produced matches and the unfilled
// remainder of taker.
//
// crosses reports whether the taker's price is willing to trade at the
// resting price level. For market orders, pass a function that always
// returns true.
func matchAgainstBook(
	book *[]MatchEngineEntry,
	taker Order,
	crosses func(takerPrice, restingPrice decimal.Decimal) bool,
	makeMatch func(taker, resting Order, price, volume decimal.Decimal) Match,
) ([]Match, decimal.Decimal) {
	var matches []Match
	remain := taker.Volume

	priceIdx := 0
	for priceIdx < len(*book) && remain.GreaterThan(decimal.Zero) {
		level := (*book)[priceIdx]
		if !crosses(taker.Price, level.Price) {
			break
		}

		// price-time priority: oldest order at this level matches first
		orderIdx := 0
		for orderIdx < len(level.Orders) && remain.GreaterThan(decimal.Zero) {
			resting := level.Orders[orderIdx]
			fill := decimal.Min(remain, resting.Volume)

			matches = append(matches, makeMatch(taker, resting, level.Price, fill))
			remain = remain.Sub(fill)

			if fill.Equal(resting.Volume) {
				// resting fully filled; drop it and keep orderIdx where it is
				level.Orders = slices.Delete(level.Orders, orderIdx, orderIdx+1)
			} else {
				// partial fill; reduce and advance
				level.Orders[orderIdx].Volume = resting.Volume.Sub(fill)
				orderIdx++
			}
		}

		(*book)[priceIdx] = level

		if len(level.Orders) == 0 {
			*book = slices.Delete(*book, priceIdx, priceIdx+1)
		} else {
			priceIdx++
		}
	}

	return matches, remain
}

func buyCrosses(takerPrice, restingPrice decimal.Decimal) bool {
	return takerPrice.GreaterThanOrEqual(restingPrice)
}

func sellCrosses(takerPrice, restingPrice decimal.Decimal) bool {
	return takerPrice.LessThanOrEqual(restingPrice)
}

func alwaysCrosses(_, _ decimal.Decimal) bool { return true }

func makeBuyMatch(taker, resting Order, price, volume decimal.Decimal) Match {
	return Match{BuyId: taker.ID, SellId: resting.ID, Price: price, Volume: volume}
}

func makeSellMatch(taker, resting Order, price, volume decimal.Decimal) Match {
	return Match{BuyId: resting.ID, SellId: taker.ID, Price: price, Volume: volume}
}

func handleLimitOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match
	var remain decimal.Decimal

	switch newOrder.Side {
	case BUY:
		matches, remain = matchAgainstBook(&orderbook.Sell, newOrder, buyCrosses, makeBuyMatch)
	case SELL:
		matches, remain = matchAgainstBook(&orderbook.Buy, newOrder, sellCrosses, makeSellMatch)
	default:
		log.Warn("unexpected Side: ", newOrder.Side)
		return nil
	}

	if remain.GreaterThan(decimal.Zero) {
		newOrder.Volume = remain
		orderbook.Add(newOrder)
	}

	return matches
}

func handleMarketOrder(orderbook *Orderbook, newOrder Order) []Match {
	var matches []Match
	var remain decimal.Decimal

	switch newOrder.Side {
	case BUY:
		matches, remain = matchAgainstBook(&orderbook.Sell, newOrder, alwaysCrosses, makeBuyMatch)
	case SELL:
		matches, remain = matchAgainstBook(&orderbook.Buy, newOrder, alwaysCrosses, makeSellMatch)
	default:
		log.Warn("unexpected Side: ", newOrder.Side)
		return nil
	}

	if remain.GreaterThan(decimal.Zero) {
		log.Warn("unfilled market order; remainder dropped: ", newOrder)
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

func CancelOrder(orderbook *Orderbook, targetOrderId int64) error {
	for idx, matchEngineEntry := range orderbook.Buy {
		for i, order := range matchEngineEntry.Orders {
			if order.ID == targetOrderId {
				if len(matchEngineEntry.Orders) == 1 {
					// remove the entry if it get emptied
					orderbook.Buy = slices.Delete(orderbook.Buy, idx, idx+1)
				} else {
					// update the volume
					orderbook.Buy[idx].Orders = slices.Delete(orderbook.Buy[idx].Orders, i, i+1)
				}
				return nil
			}
		}
	}
	for idx, matchEngineEntry := range orderbook.Sell {
		for i, order := range matchEngineEntry.Orders {
			if order.ID == targetOrderId {
				if len(matchEngineEntry.Orders) == 1 {
					// remove the entry if it get emptied
					orderbook.Sell = slices.Delete(orderbook.Sell, idx, idx+1)
				} else {
					// update the volume
					orderbook.Sell[idx].Orders = slices.Delete(orderbook.Sell[idx].Orders, i, i+1)
				}
				return nil
			}
		}
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
