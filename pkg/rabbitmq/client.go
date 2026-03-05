package rabbitmq

import (
	"log"

	"github.com/booking-show/booking-show-api/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

var Conn *amqp.Connection
var Channel *amqp.Channel

func ConnectRabbitMQ(cfg *config.Config) {
	conn, err := amqp.Dial(cfg.RabbitMQUrl)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	Conn = conn

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a RabbitMQ channel: %v", err)
	}
	Channel = ch

	log.Println("RabbitMQ connected successfully!")
}

// CloseRabbitMQ close conn and channel
func CloseRabbitMQ() {
	if Channel != nil {
		Channel.Close()
	}
	if Conn != nil {
		Conn.Close()
	}
}
