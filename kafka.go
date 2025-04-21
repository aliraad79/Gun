package main

import (
	"encoding/json"
	"math/rand"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

var (
	broker        = "localhost:9092"
	groupId       = "groupId" + string(rand.Intn(100000))
	NewOrderTopic = "NewOrder"
)

func startConsumer(wg *sync.WaitGroup, msgChan chan Order) {
	defer wg.Done()

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": broker,
		"group.id":          groupId,
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		panic(err)
	}

	defer c.Close()

	c.Subscribe(NewOrderTopic, nil)
	log.Info("Start subscribing")

	for {
		msg, err := c.ReadMessage(-1)
		if err == nil {
			log.Debug("Message on ", msg.TopicPartition, string(msg.Value))

			var order Order
			err := json.Unmarshal(msg.Value, &order)
			if err != nil {
				log.Error("Error unmarshalling:", err)
				continue
			}
			msgChan <- order
		} else {
			log.Error("Consumer error: ", err, msg)
		}
	}
}
