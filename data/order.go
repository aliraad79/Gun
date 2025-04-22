package data

import "github.com/shopspring/decimal"

type Side string

const (
	BUY  Side = "buy"
	SELL Side = "sell"
)

type Order struct {
	ID     int64           `json:"id"`
	Symbol string          `json:"symbol"`
	Side   Side            `json:"side"`
	Price  decimal.Decimal `json:"price"`
	Volume decimal.Decimal `json:"volume"`
}
