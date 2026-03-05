package rabbitmq

import (
	"encoding/json"
	"log"
)

type PaymentEvent struct {
	OrderID       string `json:"order_id"`
	Gateway       string `json:"gateway"`
	TransactionID string `json:"transaction_id"`
}

type PaymentProcessFunc func(orderID, gateway, transactionID string) error

func StartPaymentWorker(processFunc PaymentProcessFunc) {
	q, err := Channel.QueueDeclare(
		"payment.success",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue: %v", err)
	}

	msgs, err := Channel.Consume(
		q.Name,
		"",    // consumer
		false, // auto-ack — false để manual ack, đảm bảo không mất message
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer: %v", err)
	}

	log.Println("🐇 RabbitMQ Worker Listening to payment.success...")

	go func() {
		for d := range msgs {
			var event PaymentEvent
			if err := json.Unmarshal(d.Body, &event); err != nil {
				log.Printf("Failed to unmarshal payment event: %v", err)
				d.Nack(false, false)
				continue
			}

			if err := processFunc(event.OrderID, event.Gateway, event.TransactionID); err != nil {
				log.Printf("Failed to process payment success for order %s: %v", event.OrderID, err)
				d.Nack(false, true) // Requeue nếu lỗi
				continue
			}

			log.Printf("✅ Payment [%s] processed for order: %s (txn: %s)", event.Gateway, event.OrderID, event.TransactionID)
			d.Ack(false)
		}
	}()
}
