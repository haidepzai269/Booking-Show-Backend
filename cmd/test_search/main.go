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
	
	req := service.SearchMoviesReq{
		Query: "phim anh hùng",
		Limit: 20,
	}

	fmt.Printf("Searching for: '%s' using MovieService.SearchMovies...\n", req.Query)
	movies, err := movieSvc.SearchMovies(req)
	if err != nil {
		log.Fatalf("Search failed: %v", err)
	}

	fmt.Printf("Found %d movies:\n", len(movies))
	for _, m := range movies {
		fmt.Printf("- [%d] %s\n", m.ID, m.Title)
	}
}
