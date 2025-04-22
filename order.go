package main

import (
	"errors"
	"os"
	"strings"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

type Order struct {
	ID     int64           `json:"id"`
	Symbol string          `json:"symbol"`
	Side   Side            `json:"side"`
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}

type Side string

const (
	BUY  Side = "buy"
	SELL Side = "sell"
)

type Match struct {
	BuyId   int64           `json:"buy_id"`
	SellId  int64           `json:"sell_id"`
	MatchId int64           `json:"match_id"`
	Price   decimal.Decimal `json:"price"`
	Volume  decimal.Decimal `json:"volume"`
}

type MatchEngineEntry struct {
	Price  decimal.Decimal
	Orders []Order
}

type Orderbook struct {
	Buy    []MatchEngineEntry
	Sell   []MatchEngineEntry
	Symbol string
}

func (orderbook *Orderbook) add(order Order) {
	switch order.Side {
	case BUY:
		{
			lastPirce := decimal.RequireFromString("100000000000000")
			for idx, entry := range orderbook.Buy {
				if entry.Price.LessThan(order.Price) && order.Price.LessThan(lastPirce) {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.Buy = append(orderbook.Buy[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Buy[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					orderbook.Buy[idx].Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}
			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.Buy = append(orderbook.Buy, newEntry)
		}
	case SELL:
		{
			lastPirce := decimal.Zero
			for idx, entry := range orderbook.Sell {
				if entry.Price.GreaterThan(order.Price) && order.Price.GreaterThan(lastPirce) {
					newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
					orderbook.Sell = append(orderbook.Sell[:idx], append([]MatchEngineEntry{newEntry}, orderbook.Sell[idx:]...)...)
					return
				} else if entry.Price == order.Price {
					orderbook.Sell[idx].Orders = append(entry.Orders, order)
					return
				}
				lastPirce = entry.Price
			}

			newEntry := MatchEngineEntry{Orders: []Order{order}, Price: order.Price}
			orderbook.Sell = append(orderbook.Sell, newEntry)
		}
	default:
		log.Error("unexpected main.Side: ", order.Side)
	}
}

var ErrNotValidSymbol = errors.New("item not found")

func createOrderbooks(symbol string) (*Orderbook, error) {
	supported_symbols := os.Getenv("SUPPORTED_SYMBOLS")
	symbols := strings.Split(supported_symbols, ",")

	if contains(symbols, symbol) {
		return &Orderbook{Symbol: symbol}, nil
	}
	return nil, ErrNotValidSymbol
}

func loadOrFetchOrderbook(memory map[string]*Orderbook, symbol string) (*Orderbook, error) {
	_, exists := memory[symbol]
	if exists {
		return memory[symbol], nil
	} else {
		var err error

		orderbook := loadOrderbook(symbol)
		if orderbook == nil {
			orderbook, err = createOrderbooks(symbol)
		}
		log.Warn("pp ", orderbook)
		memory[symbol] = orderbook
		return orderbook, err
	}
}
