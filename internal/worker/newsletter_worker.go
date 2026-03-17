package worker

import (
	"encoding/json"
	"log"

	"github.com/booking-show/booking-show-api/pkg/rabbitmq"
)

type NewsletterNotification struct {
	UserID  int    `json:"user_id"`
	Email   string `json:"email"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

func StartNewsletterWorker() {
	if rabbitmq.Channel == nil {
		log.Println("⚠️ [Newsletter Worker] RabbitMQ Channel is nil, skipping StartNewsletterWorker.")
		return
	}

	q, err := rabbitmq.Channel.QueueDeclare(
		"promotion_notifications",
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare a queue for newsletter: %v", err)
		return
	}

	msgs, err := rabbitmq.Channel.Consume(
		q.Name,
		"",    // consumer
		false, // auto-ack (false for manual ack)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Printf("Failed to register a consumer for newsletter: %v", err)
		return
	}

	log.Println("📬 RabbitMQ Worker Listening to promotion_notifications...")

	go func() {
		for d := range msgs {
			var notification NewsletterNotification
			if err := json.Unmarshal(d.Body, &notification); err != nil {
				log.Printf("Failed to unmarshal newsletter notification: %v", err)
				d.Nack(false, false)
				continue
			}

			// Mô phỏng gửi email (Thực tế sẽ gọi EmailService)
			log.Printf("📧 [NEWSLETTER_WORKER] Sending email to %s: %s", notification.Email, notification.Message)
			
			// Giả định gửi thành công
			d.Ack(false)
		}
	}()
}
