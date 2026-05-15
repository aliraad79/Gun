package main

import "github.com/aliraad79/Gun/models"

// Command tags a message coming off the Kafka topic. The Kafka consumer
// translates the message key into one of these and forwards it to main.go,
// which dispatches to the market.Registry.
type Command string

const (
	NEW_ORDER_CMD      Command = "new_order"
	CANCEL_ORDER_CMD   Command = "cancel_order"
	START_LOADTEST_CMD Command = "start_loadtest"
	END_LOADTEST_CMD   Command = "end_loadtest"
)

// Instrument is the routed unit of work between the Kafka consumer and the
// dispatcher in main.go.
type Instrument struct {
	Command Command
	Value   models.Order
}
