package matchEngine_test

import (
	"testing"

	"github.com/aliraad79/Gun/matchEngine"
	"github.com/aliraad79/Gun/models"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestMatchAndAddNewOrder_BuyLimitOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.SELL, Price: decimal.NewFromInt(10000)},
				},
			},
		},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     2,
		Type:   models.LIMIT,
		Side:   models.BUY,
		Volume: decimal.NewFromFloat(1),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 1)
	assert.Equal(t, int64(2), matches[0].BuyId)
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, decimal.NewFromFloat(10000), matches[0].Price)
	assert.Equal(t, decimal.NewFromFloat(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMultipleMatchAndAddNewOrder_BuyLimitOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.SELL, Price: decimal.NewFromInt(10000)},
					{ID: 2, Volume: decimal.NewFromFloat(1), Side: models.SELL, Price: decimal.NewFromInt(10000)},
					{ID: 3, Volume: decimal.NewFromFloat(1), Side: models.SELL, Price: decimal.NewFromInt(10000)},
				},
			},
		},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     4,
		Symbol: "BTC_USDT",
		Type:   models.LIMIT,
		Side:   models.BUY,
		Volume: decimal.NewFromFloat(3),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 3)
	for _, match := range matches {
		assert.Equal(t, decimal.NewFromFloat(10000), match.Price)
		assert.Equal(t, decimal.NewFromFloat(1), match.Volume)
	}
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMatchAndAddNewOrder_SellLimitOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Buy: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.BUY, Price: decimal.NewFromInt(10000)},
				},
			},
		},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     2,
		Type:   models.LIMIT,
		Side:   models.SELL,
		Volume: decimal.NewFromFloat(1),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].BuyId)
	assert.Equal(t, int64(2), matches[0].SellId)
	assert.Equal(t, decimal.NewFromFloat(10000), matches[0].Price)
	assert.Equal(t, decimal.NewFromFloat(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMultipleMatchAndAddNewOrder_SellLimitOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Buy: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.BUY, Price: decimal.NewFromInt(10000)},
					{ID: 2, Volume: decimal.NewFromFloat(1), Side: models.BUY, Price: decimal.NewFromInt(10000)},
					{ID: 3, Volume: decimal.NewFromFloat(1), Side: models.BUY, Price: decimal.NewFromInt(10000)},
				},
			},
		},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     4,
		Symbol: "BTC_USDT",
		Type:   models.LIMIT,
		Side:   models.SELL,
		Volume: decimal.NewFromFloat(3),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 3)
	for _, match := range matches {
		assert.Equal(t, decimal.NewFromFloat(10000), match.Price)
		assert.Equal(t, decimal.NewFromFloat(1), match.Volume)
	}
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestCancelOrder_Success(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Buy: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.BUY},
				},
			},
		},
	}

	targetOrderId := int64(1)

	err := matchEngine.CancelOrder(orderbook, targetOrderId)

	assert.NoError(t, err)
	assert.Empty(t, orderbook.Buy)
}

func TestCancelInvalidOrder_Success(t *testing.T) {
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Buy: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{ID: 1, Volume: decimal.NewFromFloat(1), Side: models.BUY},
				},
			},
		},
	}

	targetOrderId := int64(2)

	err := matchEngine.CancelOrder(orderbook, targetOrderId)

	assert.Error(t, err, matchEngine.ErrCancelOrderFailed)
	assert.Equal(t, len(orderbook.Buy), 1)
	assert.Equal(t, len(orderbook.Buy[0].Orders), 1)
}

func TestMatchAndAddNewOrder_sellMarketOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Buy: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{
						ID:           1,
						Volume:       decimal.NewFromFloat(1),
						Side:         models.BUY,
						Price:        decimal.NewFromInt(10000),
						Type:         models.LIMIT,
						Symbol:       "BTC_USDT",
						TriggerPrice: decimal.Decimal{},
					},
				},
			},
		},
		Sell:              []models.MatchEngineEntry{},
		ConditionalOrders: []models.Order{},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     2,
		Type:   models.MARKET,
		Side:   models.SELL,
		Volume: decimal.NewFromFloat(1),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].BuyId)
	assert.Equal(t, int64(2), matches[0].SellId)
	assert.Equal(t, decimal.NewFromFloat(10000), matches[0].Price)
	assert.Equal(t, decimal.NewFromFloat(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}

func TestMatchAndAddNewOrder_buyMarketOrder(t *testing.T) {
	// Setup a simple orderbook
	orderbook := &models.Orderbook{
		Symbol: "BTC_USDT",
		Sell: []models.MatchEngineEntry{
			{
				Price: decimal.NewFromFloat(10000),
				Orders: []models.Order{
					{
						ID:           1,
						Volume:       decimal.NewFromFloat(1),
						Side:         models.SELL,
						Price:        decimal.NewFromInt(10000),
						Type:         models.LIMIT,
						Symbol:       "BTC_USDT",
						TriggerPrice: decimal.Decimal{},
					},
				},
			},
		},
		Buy:              []models.MatchEngineEntry{},
		ConditionalOrders: []models.Order{},
	}

	// Create a new buy limit order
	newOrder := models.Order{
		ID:     2,
		Type:   models.MARKET,
		Side:   models.BUY,
		Volume: decimal.NewFromFloat(1),
		Price:  decimal.NewFromFloat(10000),
	}

	// Act
	matches := matchEngine.MatchAndAddNewOrder(orderbook, newOrder)

	// Assert
	assert.Len(t, matches, 1)
	assert.Equal(t, int64(1), matches[0].SellId)
	assert.Equal(t, int64(2), matches[0].BuyId)
	assert.Equal(t, decimal.NewFromFloat(10000), matches[0].Price)
	assert.Equal(t, decimal.NewFromFloat(1), matches[0].Volume)
	assert.Empty(t, orderbook.Sell)
	assert.Empty(t, orderbook.Buy)
}