package main

import (
	"fmt"
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/service"
)

func main() {
	cfg := config.LoadEnv()
	aiSvc := service.NewAIService(cfg.GroqAPIKey, "")

	query := "phim anh hùng"
	moviesMeta := `ID: 2 | Tên: Avengers: Doomsday | Thể loại: Siêu anh hùng | Mô tả: Các siêu anh hùng Marvel tập hợp...
ID: 1 | Tên: Inception | Thể loại: Hành động | Mô tả: Một kẻ trộm có khả năng đi vào giấc mơ...
ID: 5 | Title: Spider-man: No Way Home | Genres: Siêu anh hùng, Hành động | Description: Peter Parker đối mặt với đa vũ trụ...`

	fmt.Printf("Testing LLM Search with query: '%s'\n", query)
	ids, err := aiSvc.AnalyzeSearchQuery(query, moviesMeta)
	if err != nil {
		log.Fatalf("AI Error: %v", err)
	}

	fmt.Printf("AI matched IDs: %v\n", ids)
}
