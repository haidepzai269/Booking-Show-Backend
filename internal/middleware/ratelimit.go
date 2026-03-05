package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
)

// RateLimiter dùng Sliding Window (giới hạn theo IP hoặc UserID tùy biến)
// 10 requests / 60 seconds
func RateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		if userID, exists := c.Get("userID"); exists {
			clientIP = fmt.Sprintf("user:%v", userID)
		}

		key := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), clientIP)
		now := time.Now().UnixNano()
		windowStart := now - window.Nanoseconds()

		ctx := context.Background()

		// Xoá các request cũ ngoài cửa sổ thời gian
		redis.Client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", windowStart))

		// Đếm số lượng request hiện tại
		count, err := redis.Client.ZCard(ctx, key).Result()
		if err != nil {
			c.Next() // Bypass or error handling
			return
		}

		if count >= int64(limit) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "Too many requests, please try again later",
				"code":    "RATE_LIMIT_EXCEEDED",
			})
			c.Abort()
			return
		}

		// Thêm request hiện tại vào sorted set
		redis.Client.ZAdd(ctx, key, goredis.Z{Score: float64(now), Member: now})
		redis.Client.Expire(ctx, key, window)

		c.Next()
	}
}
