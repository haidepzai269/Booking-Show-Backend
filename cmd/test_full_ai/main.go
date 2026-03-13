package main

import (
	"fmt"
	"log"
	"time"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
)

func main() {
	start := time.Now()
	cfg := config.LoadEnv()
	repository.ConnectDB(cfg)

	movieSvc := &service.MovieService{}
	meta, err := movieSvc.GetMoviesMeta()
	if err != nil {
		log.Fatalf("Error getting meta: %v", err)
	}

	fmt.Printf("Meta fetched in %v (len: %d)\n", time.Since(start), len(meta))

	aiStart := time.Now()
	aiSvc := service.NewAIService(cfg.GroqAPIKey, "")
	query := "phim anh hùng"
	fmt.Printf("Analyzing query: '%s'...\n", query)
	
	ids, err := aiSvc.AnalyzeSearchQuery(query, meta)
	if err != nil {
		log.Fatalf("AI Error: %v", err)
	}

	fmt.Printf("AI matched IDs: %v (took %v)\n", ids, time.Since(aiStart))
	fmt.Printf("Total time: %v\n", time.Since(start))
}
