package main

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/aliraad79/Gun/models"
	"github.com/aliraad79/Gun/utils"
	log "github.com/sirupsen/logrus"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

const (
	BROKER_URL = "localhost:9092"
	GROUP_ID   = "groupId"
	TOPIC      = "main_topic"
)

func startConsumer(wg *sync.WaitGroup, msgChan chan Instrument) {
	defer wg.Done()

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": utils.GetEnvOrDefault("KAFKA_BROKER_URL", BROKER_URL),
		"group.id":          utils.GetEnvOrDefault("KAFKA_GROUP_ID", GROUP_ID),
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		panic(err)
	}

	defer c.Close()

	c.Subscribe(TOPIC, nil)
	log.Info("Start subscribing")

	for {
		if msg, err := c.ReadMessage(-1); err != nil {
			log.Error("Consumer error: ", err, msg)
		} else {
			log.Debug("Message on ", msg.TopicPartition, string(msg.Value))

			key := string(msg.Key)
			if strings.HasPrefix(key, "create_") {
				var order models.Order
				err := json.Unmarshal(msg.Value, &order)
				if err != nil {
					log.Error("Error unmarshalling:", err)
					continue
				}
				msgChan <- Instrument{Command: NEW_ORDER_CMD, Value: order}
			} else if strings.HasPrefix(key, "cancel_") {
				var order models.Order
				err := json.Unmarshal(msg.Value, &order)
				if err != nil {
					log.Error("Error unmarshalling: ", err, " value: ", msg.Value)
					continue
				}
				msgChan <- Instrument{Command: CANCEL_ORDER_CMD, Value: order}
			} else if key == "start_loadtest" {
				msgChan <- Instrument{Command: START_LOADTEST_CMD}
			} else if key == "end_loadtest" {
				msgChan <- Instrument{Command: END_LOADTEST_CMD}
			} else {
				log.Error("Not a valid key", key)
			}
		}
	}
}
