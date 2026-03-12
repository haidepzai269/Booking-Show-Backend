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
		log.Printf("⚠️  Failed to parse Redis URL: %v — Server sẽ chạy không có cache Redis.", err)
		return
	}

	Client = redis.NewClient(opts)

	pong, err := Client.Ping(Ctx).Result()
	if err != nil {
		// Không Fatalf — cho phép server chạy không có Redis (cache bị bỏ qua)
		log.Printf("⚠️  Failed to connect to Redis: %v — Server vẫn chạy bình thường, không có cache.", err)
		Client = nil // Đặt nil để các nơi check `if redispkg.Client != nil` sẽ bỏ qua cache
		return
	}
	log.Println("✅ Redis connected successfully!", pong)
}

