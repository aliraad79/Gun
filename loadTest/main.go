package main

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

type Order struct {
	ID     int64   `json:"id"`
	Symbol string  `json:"symbol"`
	Side   string  `json:"side"`
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

func main() {
	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": "localhost:9092"})
	if err != nil {
		panic(err)
	}

	topic := "NewOrder"
	
	startTime := time.Now()
	log.Println("Start spamming with fake orders")
	for i := 1; i < 2; i++ {
		order := Order{ID: int64(i), Symbol: "BTC_USDT", Side:"buy", Price: float64(100 - i), Volume: 10}
		json_order, err := json.Marshal(order)
		if err != nil {
			log.Println("Error marshalling:", err)
			return
		}
		p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(strconv.FormatInt(order.ID, 10)),
			Value:          json_order,
		}, nil)
	}

	for i := 1; i < 2; i++ {
		order := Order{ID: int64(i), Symbol: "BTC_USDT", Side:"sell", Price: float64(100 - i), Volume: 5}
		json_order, err := json.Marshal(order)
		if err != nil {
			log.Println("Error marshalling:", err)
			return
		}
		p.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
			Key:            []byte(strconv.FormatInt(order.ID, 10)),
			Value:          json_order,
		}, nil)
	}
	p.Flush(500)
	log.Println("All messaged send to topic", topic, time.Since(startTime))

}
