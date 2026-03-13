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
	if err := repository.DB.Preload("Genres").Find(&movies).Error; err != nil {
		log.Fatalf("Failed to fetch movies: %v", err)
	}

	fmt.Printf("Found %d movies in DB:\n", len(movies))
	for _, m := range movies {
		genres := ""
		for _, g := range m.Genres {
			genres += g.Name + ", "
		}
		fmt.Printf("ID: %d | Title: %s | Active: %t | Genres: %s | Description: %.100s...\n", m.ID, m.Title, m.IsActive, genres, m.Description)
	}
}
