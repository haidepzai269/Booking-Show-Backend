package repository

import (
	"log"

	"github.com/booking-show/booking-show-api/internal/model"
)

// MigrateDB create tables
func MigrateDB() {
	err := DB.AutoMigrate(
		&model.User{},
		&model.Cinema{},
		&model.Room{},
		&model.Seat{},
		&model.Genre{},
		&model.Movie{},
		&model.Concession{},
		&model.Promotion{},
		&model.Showtime{},
		&model.ShowtimeSeat{},
		&model.Order{},
		&model.OrderConcession{},
		&model.OrderSeat{},
		&model.Payment{},
		&model.Ticket{},
		&model.Person{},
		&model.RefundRequest{},
		&model.FAQLog{},
		&model.Campaign{},
	)

	if err != nil {
		log.Fatalf("Failed to auto migrate database: %v", err)
	}

	log.Println("Database migration completed!")
}
