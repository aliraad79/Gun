package models

import "github.com/shopspring/decimal"

type Match struct {
	BuyId   int64           `json:"buy_id"`
	SellId  int64           `json:"sell_id"`
	MatchId int64           `json:"match_id"`
	Price   decimal.Decimal `json:"price"`
	Volume  decimal.Decimal `json:"volume"`
}
