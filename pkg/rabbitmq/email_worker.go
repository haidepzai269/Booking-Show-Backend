package rabbitmq

import (
	"encoding/json"
	"log"
)

type EmailEvent struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type EmailProcessFunc func(email, token string) error

func StartEmailWorker(processFunc EmailProcessFunc) {
	q, err := Channel.QueueDeclare(
		"email.send_magic_link",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to declare a queue for email: %v", err)
	}

	msgs, err := Channel.Consume(
		q.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to register a consumer for email: %v", err)
	}

	log.Println("📧 RabbitMQ Worker Listening to email.send_magic_link...")

	go func() {
		for d := range msgs {
			var event EmailEvent
			if err := json.Unmarshal(d.Body, &event); err != nil {
				log.Printf("Failed to unmarshal email event: %v", err)
				d.Nack(false, false)
				continue
			}

			if err := processFunc(event.Email, event.Token); err != nil {
				log.Printf("Failed to send email to %s: %v", event.Email, err)
				// Tùy theo logic nghiệp vụ, có thể requeue hoặc bỏ qua
				// Vì email thường bị lỗi ngẫu nhiên do network của SMTP nên ta cho requeue
				d.Nack(false, true)
				continue
			}

			log.Printf("✅ Email sent to %s via RabbitMQ", event.Email)
			d.Ack(false)
		}
	}()
}
