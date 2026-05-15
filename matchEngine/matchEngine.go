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

// Result describes the outcome of a single MatchAndAddNewOrder call.
//
// An order is *accepted* (Accepted == true) if the engine took ownership
// of it: it either matched in full, matched in part with the remainder
// resting on the book (or dropped for IOC / market), or fully rested. A
// rejected order produced no state change at all — no matches, no resting
// remainder, no journal entry. Reason carries a short human-readable tag
// useful for client error messages and audit logs.
type Result struct {
	Matches  []Match
	Accepted bool
	Reason   string
}

// Rejection reason constants. Stable strings; clients can match on them.
const (
	RejectFOKUnfillable    = "fok_not_fully_fillable"
	RejectPostOnlyCrossed  = "post_only_would_cross"
	RejectInvalidOrder     = "invalid_order"
	RejectUnsupportedType  = "unsupported_order_type"
	RejectUnsupportedTIF   = "unsupported_time_in_force"
)

// HandleConditionalOrders walks the orderbook's conditional queue and
// promotes any orders whose trigger has been hit by one of the just-produced
// matches.
func HandleConditionalOrders(orderbook *Orderbook, lastMatches []Match) []Match {
	var matches []Match
	for _, order := range orderbook.ConditionalOrders {
		for _, match := range lastMatches {
			if order.IsTriggered(match.Price) {
				r := handleLimitOrder(orderbook, order)
				matches = append(matches, r.Matches...)
			}
		}
	}
	return matches
}

// MatchAndAddNewOrder processes one new order against the book and returns
// the produced matches plus an accept/reject status. The taker order is
// Normalized first so callers don't have to.
func MatchAndAddNewOrder(orderbook *Orderbook, newOrder Order) Result {
	newOrder.Normalize()

	switch newOrder.Type {
	case LIMIT:
		return handleLimitOrder(orderbook, newOrder)
	case MARKET:
		return handleMarketOrder(orderbook, newOrder)
	case STOP_LIMIT:
		return handleStopLimitOrder(orderbook, newOrder)
	default:
		return Result{Accepted: false, Reason: fmt.Sprintf("%s: %v", RejectUnsupportedType, newOrder.Type)}
	}
}

func handleLimitOrder(orderbook *Orderbook, newOrder Order) Result {
	// Post-only: reject before any matching if the order would cross.
	if newOrder.TimeInForce == PostOnly && wouldCross(orderbook, newOrder) {
		return Result{Accepted: false, Reason: RejectPostOnlyCrossed}
	}

	// FOK: pre-check the available crossing volume; reject if the order
	// cannot be filled in full in one pass.
	if newOrder.TimeInForce == FOK && !canFullyFill(orderbook, newOrder) {
		return Result{Accepted: false, Reason: RejectFOKUnfillable}
	}

	matches, remain := orderbook.MatchTaker(newOrder, false)

	switch newOrder.TimeInForce {
	case IOC, FOK:
		// IOC drops unfilled remainder; FOK can't reach this branch with
		// any unfilled remainder (pre-check ensures full fill), but we
		// guard against drift anyway.
		_ = remain
	default: // GTC and PostOnly (which got here only after passing the
		// post-only pre-check above) rest the remainder.
		if remain.IsPositive() {
			newOrder.Volume = remain
			orderbook.Add(newOrder)
		}
	}

	return Result{Matches: matches, Accepted: true}
}

func handleMarketOrder(orderbook *Orderbook, newOrder Order) Result {
	// Market orders never rest; any unfilled remainder is silently dropped.
	matches, _ := orderbook.MatchTaker(newOrder, true)
	return Result{Matches: matches, Accepted: true}
}

func handleStopLimitOrder(orderbook *Orderbook, order Order) Result {
	// TODO: source the reference price from the last-trade tracker.
	if order.IsTriggered(ZeroPx) {
		return handleLimitOrder(orderbook, order)
	}
	orderbook.AddConditionalOrder(order)
	return Result{Accepted: true}
}

// wouldCross reports whether a limit order would take liquidity if matched
// right now. Used by post-only TIF.
func wouldCross(orderbook *Orderbook, taker Order) bool {
	var top *MatchEngineEntry
	switch taker.Side {
	case BUY:
		if len(orderbook.Sell) == 0 {
			return false
		}
		top = orderbook.Sell[0]
		return taker.Price.Gte(top.Price)
	case SELL:
		if len(orderbook.Buy) == 0 {
			return false
		}
		top = orderbook.Buy[0]
		return taker.Price.Lte(top.Price)
	}
	return false
}

// canFullyFill walks the opposite-side ladder summing volume at crossing
// price levels (without mutating) and returns whether the cumulative
// crossing volume meets or exceeds the taker's volume. Used by FOK TIF.
func canFullyFill(orderbook *Orderbook, taker Order) bool {
	var book []*MatchEngineEntry
	var crosses func(Px) bool
	switch taker.Side {
	case BUY:
		book = orderbook.Sell
		crosses = func(p Px) bool { return taker.Price.Gte(p) }
	case SELL:
		book = orderbook.Buy
		crosses = func(p Px) bool { return taker.Price.Lte(p) }
	default:
		return false
	}

	need := taker.Volume
	for _, level := range book {
		if !crosses(level.Price) {
			break
		}
		for n := level.Orders.Head(); n != nil; n = n.Next {
			need = need.Sub(n.Order.Volume)
			if !need.IsPositive() {
				return true
			}
		}
	}
	return false
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
