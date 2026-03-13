package main

import (
	"fmt"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	var movies []model.Movie
	repository.DB.Find(&movies)

	fmt.Printf("Total movies in DB: %d\n", len(movies))
	for _, m := range movies {
		fmt.Printf("ID: %d | Title: %s | Active: %t\n", m.ID, m.Title, m.IsActive)
	}
}
