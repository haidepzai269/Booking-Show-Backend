package main

import (
	"log"
	"net/http"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/cron"
	"github.com/booking-show/booking-show-api/internal/handler"
	"github.com/booking-show/booking-show-api/internal/middleware"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/booking-show/booking-show-api/pkg/cloudinary"
	"github.com/booking-show/booking-show-api/pkg/rabbitmq"
	"github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/booking-show/booking-show-api/pkg/sse"

	"github.com/gin-gonic/gin"
)

func main() {
	// 1. Load configuration
	cfg := config.LoadEnv()

	// 2. Initialize Connections
	repository.ConnectDB(cfg)
	repository.MigrateDB()
	redis.ConnectRedis(cfg)
	go sse.StartSubscriber()
	rabbitmq.ConnectRabbitMQ(cfg)
	defer rabbitmq.CloseRabbitMQ()
	// Khởi động worker ngầm lắng nghe log và email
	ticketSvc := &service.TicketService{}
	rabbitmq.StartPaymentWorker(ticketSvc.ProcessPaymentSuccess)

	emailSvc := service.NewEmailService(cfg)
	rabbitmq.StartEmailWorker(emailSvc.SendMagicLink)

	// Khởi động cronjob 1 phút check 1 lần đễ dọn dẹp các khoản thanh toán quá 10p
	cron.StartOrderCleanupCronjob()

	cloudinary.ConnectCloudinary(cfg)

	// 3. Setup Gin Router
	r := gin.Default()
	r.Use(middleware.CORSMiddleware())
	handler.SetupRouter(r, cfg)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"db":      "connected",
			"redis":   "connected",
			"rabbit":  "connected",
		})
	})

	// 4. Start Server
	log.Printf("Server is running on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
