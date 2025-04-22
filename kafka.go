package main

import (
	"encoding/json"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

const (
	BROKER_URL      = "localhost:9092"
	GROUP_ID        = "groupId"
	NEW_ORDER_TOPIC = "NewOrder"
)

func startConsumer(wg *sync.WaitGroup, msgChan chan Instrument) {
	defer wg.Done()

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": getEnvOrDefault("KAFKA_BROKER_URL", BROKER_URL),
		"group.id":          getEnvOrDefault("KAFKA_GROUP_ID", GROUP_ID),
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		panic(err)
	}

	defer c.Close()

	c.Subscribe(NEW_ORDER_TOPIC, nil)
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
			msgChan <- Instrument{Command: NEW_ORDER_CMD, Value: order}
		} else {
			log.Error("Consumer error: ", err, msg)
		}
	}
}
