package main

import (
	"fmt"
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
)

func main() {
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	movieSvc := &service.MovieService{}
	meta, err := movieSvc.GetMoviesMeta()
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("--- Metada (len: %d) ---\n", len(meta))
	fmt.Println(meta)
	fmt.Println("--- End Metada ---")
}
