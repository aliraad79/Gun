package main

import (
	"encoding/json"
	"fmt"

	log "github.com/sirupsen/logrus"

	"github.com/go-redis/redis"
)

func commitOrderBook(orderbook Orderbook) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     getEnvOrDefault("REDIS_URL", "localhost:6379"),
		Password: "",
		DB:       0,
	})

	jsonStringOrderbook, err := json.Marshal(orderbook)
	if err != nil {
		log.Error("Can't marshall for ", orderbook, " from persistance memory ", err)
		return
	}

	_, err = rdb.Set(fmt.Sprint("Orderbook_", orderbook.Symbol), []byte(jsonStringOrderbook), 0).Result()
	if err != nil {
		log.Error("Can't set value for ", orderbook.Symbol, " to persistance memory ", err)
	}
}

func loadOrderbook(symbol string) *Orderbook {
	rdb := redis.NewClient(&redis.Options{
		Addr:     getEnvOrDefault("REDIS_URL", "localhost:6379"),
		Password: "",
		DB:       0,
	})

	jsonStringOrderbook, err := rdb.Get(fmt.Sprint("Orderbook_", symbol)).Result()
	if err != nil {
		log.Error("Can't get value for ", symbol, " from persistance memory", err)
		return nil
	}

	if jsonStringOrderbook == "" {
		log.Info("No data was found in persistance memory for ", symbol)
		return nil
	}

	orderbook := Orderbook{}
	err = json.Unmarshal([]byte(jsonStringOrderbook), &orderbook)
	if err != nil {
		log.Error("Can't unmarshall for ", symbol, " from persistance memory ", err)
		return nil
	}
	log.Debug("Fetch orderbook from persistance memory ", orderbook)

	return &orderbook
}
