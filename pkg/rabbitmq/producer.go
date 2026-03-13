package rabbitmq

import (
	"context"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishMessage(queueName string, body []byte) error {
	if Channel == nil {
		log.Println("⚠️ [RabbitMQ Producer] Channel is nil, skipping PublishMessage.")
		return nil // Hoặc trả về error tùy logic, ở đây chọn im lặng để không gây crash
	}
	q, err := Channel.QueueDeclare(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare a queue: %v", err)
		return err
	}

	err = Channel.PublishWithContext(context.Background(),
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		log.Printf("Failed to publish a message: %v", err)
		return err
	}
	return nil
}
