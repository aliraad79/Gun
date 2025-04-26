package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"time"

	"fmt"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type Order struct {
	ID     int     `json:"id"`
	Symbol string  `json:"symbol"`
	Type   string  `json:"type"`
	Side   string  `json:"side"`
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

func randomPrice() float64 {
	min := 10.0
	max := 20.0

	return min + rand.Float64()*(max-min)
}

func randomVolume() float64 {
	min := 1.0
	max := 2.0

	return min + rand.Float64()*(max-min)
}

func randomSide() string {
	rand := rand.Float64()
	if rand > 0.5 {
		return "buy"
	} else {
		return "sell"
	}
}

// var coins = []string{"BTC_USDT", "ETH_USDT", "DOGE_USDT", "ADA_USDT", "BNB_USDT", "TRX_USDT", "XMR_USDT", "FIL_USDT", "FLOKI_USDT", "XAUT_USDT"}
var coins = []string{"BTC_USDT"}

func randomSymbol() string {
	return coins[rand.Intn(len(coins))]
}

var (
	TOPIC = "main_topic"
)

func main() {
	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": "localhost:9092"})
	if err != nil {
		panic(err)
	}

	startTime := time.Now()

	p.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &TOPIC, Partition: kafka.PartitionAny},
		Key:            []byte("start_loadtest"),
		Value:          []byte("{}"),
	}, nil)
	log.Println("Start spamming with fake orders")
	for i := 1; i < 5000; i++ {
		order := Order{ID: i, Symbol: randomSymbol(), Side: randomSide(), Price: randomPrice(), Volume: randomVolume(), Type: "limit"}
		if json_order, err := json.Marshal(order); err == nil {
			p.Produce(&kafka.Message{
				TopicPartition: kafka.TopicPartition{Topic: &TOPIC, Partition: kafka.PartitionAny},
				Key:            []byte("create_" + fmt.Sprint(order.ID)),
				Value:          json_order,
			}, nil)
		}
	}

	p.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &TOPIC, Partition: kafka.PartitionAny},
		Key:            []byte("end_loadtest"),
		Value:          []byte("{}"),
	}, nil)

	p.Flush(5000)
	log.Println("All messaged send to topic", TOPIC, time.Since(startTime))
	p.Close()
}
