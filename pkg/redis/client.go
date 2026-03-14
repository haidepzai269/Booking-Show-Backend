package redis

import (
	"context"
	"log"
	"time"

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

	// Tối ưu các thông số kết nối để tránh timeout khi mạng không ổn định
	opts.DialTimeout = 10 * time.Second
	opts.ReadTimeout = 10 * time.Second
	opts.PoolSize = 10
	opts.MinIdleConns = 3

	Client = redis.NewClient(opts)

	// Sử dụng context với timeout cho lệnh Ping
	ctx, cancel := context.WithTimeout(Ctx, 15*time.Second)
	defer cancel()

	pong, err := Client.Ping(ctx).Result()
	if err != nil {
		log.Printf("Warning: Failed to connect to Redis (%s): %v. Caching will be disabled.", cfg.RedisUrl, err)
		Client = nil
		return
	}
	log.Println("Redis connected successfully!", pong)
}
