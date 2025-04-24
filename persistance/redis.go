package persistance

import (
	"encoding/json"
	"fmt"

	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/utils"
	log "github.com/sirupsen/logrus"

	"github.com/go-redis/redis"
)

func RedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     utils.GetEnvOrDefault("REDIS_URL", "localhost:6379"),
		Password: "",
		DB:       0,
		PoolSize: 10,
	})
}

func CommitOrderBook(orderbook models.Orderbook, rdb *redis.Client) {
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

func LoadOrderbook(symbol string, rdb *redis.Client) *models.Orderbook {
	jsonStringOrderbook, err := rdb.Get(fmt.Sprint("Orderbook_", symbol)).Result()
	if err != nil {
		log.Error("Can't get value for ", symbol, " from persistance memory", err)
		return nil
	}

	if jsonStringOrderbook == "" {
		log.Info("No data was found in persistance memory for ", symbol)
		return nil
	}

	orderbook := models.Orderbook{}
	err = json.Unmarshal([]byte(jsonStringOrderbook), &orderbook)
	if err != nil {
		log.Error("Can't unmarshall for ", symbol, " from persistance memory ", err)
		return nil
	}
	log.Debug("Fetch orderbook from persistance memory ", orderbook)

	return &orderbook
}
