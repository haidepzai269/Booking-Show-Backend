package redis

import (
	"context"
	"log"

	"github.com/booking-show/booking-show-api/config"
	"github.com/redis/go-redis/v9"
)

var Client *redis.Client
var Ctx = context.Background()

func ConnectRedis(cfg *config.Config) {
	opts, err := redis.ParseURL(cfg.RedisUrl)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}

	Client = redis.NewClient(opts)

	pong, err := Client.Ping(Ctx).Result()
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v. Caching will be disabled.", err)
		Client = nil
		return
	}
	log.Println("Redis connected successfully!", pong)
}
