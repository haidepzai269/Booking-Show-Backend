package main

import (
	"fmt"
	"log"

	"github.com/booking-show/booking-show-api/config"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
)

func main() {
	cfg := config.LoadEnv()
	redispkg.ConnectRedis(cfg)

	if redispkg.Client == nil {
		log.Fatal("Redis client is nil")
	}

	// Xóa cache metadata
	redispkg.Client.Del(redispkg.Ctx, "movies:meta:rag")
	
	// Xóa tất cả các cache search
	iter := redispkg.Client.Scan(redispkg.Ctx, 0, "movies:search:*", 0).Iterator()
	for iter.Next(redispkg.Ctx) {
		redispkg.Client.Del(redispkg.Ctx, iter.Val())
		fmt.Printf("Deleted cache key: %s\n", iter.Val())
	}

	fmt.Println("Cleared movie search and RAG metadata cache.")
}
