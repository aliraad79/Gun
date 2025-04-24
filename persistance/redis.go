package persistance

import (
	"fmt"

	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/utils"
	log "github.com/sirupsen/logrus"

	"github.com/go-redis/redis"

	protoModels "github.com/aliraad79/Gun/models/models"
	"google.golang.org/protobuf/proto"
)

var rdb *redis.Client

func InitClient() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     utils.GetEnvOrDefault("REDIS_URL", "localhost:6379"),
		Password: "",
		DB:       0,
		PoolSize: 10,
	})
}

func CommitOrderBook(orderbook models.Orderbook) {
	bytesOrderbook, err := proto.Marshal(orderbook.ToProto())
	if err != nil {
		log.Error("Can't marshall for ", orderbook, " from persistance memory ", err)
		return
	}

	_, err = rdb.Set(fmt.Sprint("Orderbook_", orderbook.Symbol), bytesOrderbook, 0).Result()
	if err != nil {
		log.Error("Can't set value for ", orderbook.Symbol, " to persistance memory ", err)
	}
}

func LoadOrderbook(symbol string) *models.Orderbook {
	stringOrderbook, err := rdb.Get(fmt.Sprint("Orderbook_", symbol)).Result()
	if err != nil {
		log.Error("Can't get value for ", symbol, " from persistance memory", err)
		return nil
	}

	if stringOrderbook == "" {
		log.Debug("No data was found in persistance memory for ", symbol)
		return nil
	}

	odb := &protoModels.Orderbook{}
	err = proto.Unmarshal([]byte(stringOrderbook), odb)
	if err != nil {
		log.Error("Can't unmarshall for ", symbol, " from persistance memory ", err)
		return nil
	}
	orderbook := models.FromProto(*odb)

	log.Debug("Fetch orderbook from persistance memory ", orderbook)

	return orderbook
}
