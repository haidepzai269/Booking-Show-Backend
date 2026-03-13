package main

import (
	"fmt"
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	var movies []model.Movie
	if err := repository.DB.Preload("Genres").Where("is_active = ?", true).Limit(50).Find(&movies).Error; err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Fetched %d movies for Meta\n", len(movies))
	for _, m := range movies {
		fmt.Printf("ID: %d | Title: %s | Desc Len: %d\n", m.ID, m.Title, len(m.Description))
	}
}
